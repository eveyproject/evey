package com.evey.app

import android.content.Context
import android.media.AudioManager
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import kotlin.math.roundToInt

/**
 * AudioBoost - the player's >100% volume, and the device media volume its
 * 0-100% half drives. The amplification is ONE stage: EveyAudioPreamp, inside
 * our own player's audio chain (expo-video patch), with a gain glide and a
 * look-ahead limiter. The LoudnessEnhancer fallback is gone: bound to the
 * global mix or to a stale session it amplified other apps, and stacked with
 * the preamp it doubled the gain. gain is linear: 1.0 = 100%, 2.0 = 200%.
 */
class AudioBoostModule(reactContext: ReactApplicationContext) :
  ReactContextBaseJavaModule(reactContext) {

  override fun getName() = "AudioBoost"

  // setBoost and release go through the SAME queue, so they land in the order
  // JS sent them: a release run directly overtook a queued setBoost, and the
  // next film started amplified with its bar at 100%.
  @ReactMethod
  fun setBoost(gain: Double, promise: Promise) {
    reactApplicationContext.runOnUiQueueThread {
      expo.modules.video.EveyAudioPreamp.gain = gain.toFloat().coerceIn(1f, 2f)
      promise.resolve(gain > 1.0)
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
      // Read back: a fixed-volume device, or the headphone safe-volume cap,
      // leaves it lower than asked.
      promise.resolve(am.getStreamVolume(AudioManager.STREAM_MUSIC).toDouble() / max)
    } catch (e: SecurityException) {
      // A Do-Not-Disturb policy can forbid volume changes. Fail soft: the caller
      // falls back to the player-element volume rather than losing sound control.
      promise.reject("device_volume_forbidden", e.message, e)
    }
  }

  @ReactMethod
  fun release(promise: Promise) {
    reactApplicationContext.runOnUiQueueThread {
      resetGain()
      promise.resolve(null)
    }
  }

  override fun invalidate() {
    resetGain()
    super.invalidate()
  }

  // The gain is process-wide: left at 2.0 it amplified the next film.
  private fun resetGain() {
    expo.modules.video.EveyAudioPreamp.gain = 1f
  }
}
