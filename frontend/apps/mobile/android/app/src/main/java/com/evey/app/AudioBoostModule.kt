package com.evey.app

import android.content.Context
import android.media.AudioManager
import android.media.audiofx.LoudnessEnhancer
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import kotlin.math.log10
import kotlin.math.roundToInt

/**
 * AudioBoost - VLC-style volume amplification above the player's 100% ceiling.
 *
 * expo-video / ExoPlayer clamp `volume` to 1.0, so the >100% zone applies a gain
 * to the global output mix (audio session 0) with a [LoudnessEnhancer] - the
 * simple, battle-tested primitive every "volume booster" app uses. It only ADDS
 * gain (transparent when disabled), so unlike DynamicsProcessing it can never mute
 * the session. gain is linear: 1.0 = 100% (off), 2.0 = 200%. Target gain in
 * millibels = 2000*log10(gain) → 2.0 ≈ +6 dB. Disabled as soon as the user drops
 * back to <=100%; released on unmount.
 */
class AudioBoostModule(reactContext: ReactApplicationContext) :
  ReactContextBaseJavaModule(reactContext) {

  private var enhancer: LoudnessEnhancer? = null
  // The audio session the enhancer is built on. 0 = the global output mix -
  // the historical fallback, which also amplifies every OTHER app's audio.
  // VLC applies its >100% gain inside its own pipeline (libVLC setVolume up
  // to 200); ExoPlayer clamps volume at 1.0, so the closest equivalent is a
  // LoudnessEnhancer attached to OUR player's audioSessionId. expo-video does
  // not export it, but its VideoManager (same APK, pinned version) keeps the
  // live VideoPlayers in a private map - read reflectively, typed access
  // after that (VideoPlayer.player is public). Any failure falls back to the
  // global mix, i.e. exactly the old behaviour.
  private var enhancerSessionId: Int = 0

  override fun getName() = "AudioBoost"

  // appliedMilliBels is what the effect is currently set to, or null when it is
  // off. Re-arming a LIVE effect audibly breaks the stream up, so a call that
  // asks for the state we are already in must do NOTHING. The JS layer already
  // coalesces the 60-per-second stream of drag events (audioBoost.ts); this is
  // the second lock, because a caller that forgets should degrade to a no-op
  // rather than to broken sound (reported on a tablet, 2026-07-30: dragging the
  // volume down made the audio stutter).
  private var appliedMilliBels: Int? = null

  // Resolves the audio session of the currently playing expo-video player, or
  // 0 (global mix) when none can be found. Runs on the main looper: the
  // ExoPlayer was built on it and audioSessionId is looper-checked.
  private fun resolvePlayerSessionId(): Int {
    return try {
      val manager = expo.modules.video.managers.VideoManager
      val field = manager.javaClass.getDeclaredField("videoPlayersToVideoViews")
      field.isAccessible = true
      @Suppress("UNCHECKED_CAST")
      val players = (field.get(manager) as? Map<Any, Any>)?.keys ?: return 0
      val player = players.filterIsInstance<expo.modules.video.player.VideoPlayer>().lastOrNull() ?: return 0
      // media3 is expo-video's INTERNAL dependency - not on this module's
      // compile classpath - so the ExoPlayer half of the walk is reflective
      // too. Version-agnostic by the same token.
      val exo = player.javaClass.getMethod("getPlayer").invoke(player) ?: return 0
      val id = exo.javaClass.getMethod("getAudioSessionId").invoke(exo) as? Int ?: return 0
      if (id > 0) id else 0
    } catch (_: Throwable) {
      // Minified build renamed the field, expo-video moved it, player gone -
      // the global mix still works, it is just less polite to other apps.
      0
    }
  }

  @ReactMethod
  fun setBoost(gain: Double, promise: Promise) {
    // Hop to the main looper: the session id read touches the ExoPlayer, and
    // audio effects prefer living where their session does.
    reactApplicationContext.runOnUiQueueThread { applyBoost(gain, promise) }
  }

  private fun applyBoost(gain: Double, promise: Promise) {
    try {
      // Preferred path since the expo-video patch: a VLC-style multiplicative
      // preamp INSIDE our player's audio chain (EveyAudioPreamp) - plain
      // sample gain, no hidden compressor, no global-mix effect. The
      // LoudnessEnhancer below remains only as the fallback for streams the
      // processor cannot carry (non-16-bit-PCM output).
      expo.modules.video.EveyAudioPreamp.gain = gain.toFloat().coerceIn(1f, 2f)
      if (expo.modules.video.EveyAudioPreamp.engaged) {
        if (enhancer != null && appliedMilliBels != 0) {
          enhancer?.setTargetGain(0)
          appliedMilliBels = 0
        }
        promise.resolve(gain > 1.0)
        return
      }
      // At or below 100% there is nothing to amplify. If the effect was never
      // created, keep it that way; if it exists, take it to ZERO GAIN but keep
      // it enabled - a zero-gain LoudnessEnhancer is transparent, while
      // setEnabled(false)/(true) around the 100% boundary audibly broke the
      // stream up on every crossing of a volume drag (user report,
      // 2026-08-31). The effect is only truly disabled on release().
      if (gain <= 1.0) {
        val fx = enhancer
        if (fx != null && appliedMilliBels != 0) {
          fx.setTargetGain(0)
          appliedMilliBels = 0
        }
        promise.resolve(false)
        return
      }
      val milliBels = (2000.0 * log10(gain)).toInt().coerceIn(0, 3000) // cap ~+30 dB
      if (appliedMilliBels == milliBels) {
        promise.resolve(true)
        return
      }
      // Bind to the live player's session; if the session changed since the
      // effect was built (new playback = new ExoPlayer), rebuild on the new one.
      val sessionId = resolvePlayerSessionId()
      var fx = enhancer
      if (fx == null || sessionId != enhancerSessionId) {
        releaseEffect()
        fx = LoudnessEnhancer(sessionId)
        enhancer = fx
        enhancerSessionId = sessionId
      }
      fx.setTargetGain(milliBels)
      fx.setEnabled(true)
      appliedMilliBels = milliBels
      promise.resolve(true)
    } catch (e: Exception) {
      // Some devices/emulators have no global-session effect support - fail soft
      // so the player still works, just capped at 100%.
      appliedMilliBels = null
      promise.reject("audio_boost_unavailable", e.message, e)
    }
  }

  // ---------------------------------------------------------------------------
  // Device media volume. The player's 0-100% band drives THIS, not the player
  // element's own volume: the two multiply, so a device left low made the >100%
  // boost nearly inaudible (user report, 2026-07-30). See volumePolicy.ts.
  // ---------------------------------------------------------------------------

  private val audio: AudioManager?
    get() = reactApplicationContext.getSystemService(Context.AUDIO_SERVICE) as? AudioManager

  /**
   * Hardware step count of the media stream (commonly 15). The volume policy
   * uses it to write the device once per step and let the player element fade
   * the remainder - see volumePolicy.ts.
   */
  @ReactMethod
  fun getDeviceVolumeSteps(promise: Promise) {
    val am = audio
    if (am == null) {
      promise.reject("device_volume_unavailable", "no AudioManager")
      return
    }
    val max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
    if (max <= 0) {
      promise.reject("device_volume_unavailable", "stream has no range")
      return
    }
    promise.resolve(max)
  }

  /** Current media volume as a 0-1 fraction, so the bar can open on the truth. */
  @ReactMethod
  fun getDeviceVolume(promise: Promise) {
    val am = audio
    if (am == null) {
      promise.reject("device_volume_unavailable", "no AudioManager")
      return
    }
    val max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
    if (max <= 0) {
      promise.reject("device_volume_unavailable", "stream has no range")
      return
    }
    promise.resolve(am.getStreamVolume(AudioManager.STREAM_MUSIC).toDouble() / max)
  }

  /**
   * Set the media volume from a 0-1 fraction and resolve with what the device
   * ACTUALLY applied. STREAM_MUSIC is a small number of steps (commonly 15), so
   * a continuous drag snaps; returning the real value keeps the on-screen
   * percentage honest instead of showing a level the device never reached.
   *
   * Flag 0 = do not raise the system volume UI: the player draws its own bar,
   * and the OS panel over a fullscreen video is exactly the noise we avoid.
   */
  @ReactMethod
  fun setDeviceVolume(fraction: Double, promise: Promise) {
    val am = audio
    if (am == null) {
      promise.reject("device_volume_unavailable", "no AudioManager")
      return
    }
    try {
      val max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
      if (max <= 0) {
        promise.reject("device_volume_unavailable", "stream has no range")
        return
      }
      val index = (fraction.coerceIn(0.0, 1.0) * max).roundToInt().coerceIn(0, max)
      am.setStreamVolume(AudioManager.STREAM_MUSIC, index, 0)
      promise.resolve(index.toDouble() / max)
    } catch (e: SecurityException) {
      // A Do-Not-Disturb policy can forbid volume changes. Fail soft: the caller
      // falls back to the player-element volume rather than losing sound control.
      promise.reject("device_volume_forbidden", e.message, e)
    }
  }

  @ReactMethod
  fun release(promise: Promise) {
    releaseEffect()
    promise.resolve(null)
  }

  override fun invalidate() {
    releaseEffect()
    super.invalidate()
  }

  private fun releaseEffect() {
    try {
      enhancer?.release()
    } catch (_: Exception) {
    }
    enhancer = null
    appliedMilliBels = null
    enhancerSessionId = 0
  }
}
