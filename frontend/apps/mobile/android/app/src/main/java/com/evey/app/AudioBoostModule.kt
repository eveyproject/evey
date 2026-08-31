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

  override fun getName() = "AudioBoost"

  // appliedMilliBels is what the effect is currently set to, or null when it is
  // off. Re-arming a LIVE effect audibly breaks the stream up, so a call that
  // asks for the state we are already in must do NOTHING. The JS layer already
  // coalesces the 60-per-second stream of drag events (audioBoost.ts); this is
  // the second lock, because a caller that forgets should degrade to a no-op
  // rather than to broken sound (reported on a tablet, 2026-07-30: dragging the
  // volume down made the audio stutter).
  private var appliedMilliBels: Int? = null

  @ReactMethod
  fun setBoost(gain: Double, promise: Promise) {
    try {
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
      val fx = enhancer ?: LoudnessEnhancer(0).also { enhancer = it } // 0 = global mix
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
  }
}
