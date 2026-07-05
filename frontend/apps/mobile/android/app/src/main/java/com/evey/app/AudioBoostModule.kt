package com.evey.app

import android.media.audiofx.LoudnessEnhancer
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import kotlin.math.log10

/**
 * AudioBoost — VLC-style volume amplification above the player's 100% ceiling.
 *
 * expo-video / ExoPlayer clamp `volume` to 1.0, so the >100% zone applies a gain
 * to the global output mix (audio session 0) with a [LoudnessEnhancer] — the
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

  @ReactMethod
  fun setBoost(gain: Double, promise: Promise) {
    try {
      // At or below 100% there is nothing to amplify — turn the effect off so the
      // native volume path is untouched (and other apps' audio isn't boosted).
      if (gain <= 1.0) {
        enhancer?.setEnabled(false)
        promise.resolve(false)
        return
      }
      val milliBels = (2000.0 * log10(gain)).toInt().coerceIn(0, 3000) // cap ~+30 dB
      val fx = enhancer ?: LoudnessEnhancer(0).also { enhancer = it } // 0 = global mix
      fx.setTargetGain(milliBels)
      fx.setEnabled(true)
      promise.resolve(true)
    } catch (e: Exception) {
      // Some devices/emulators have no global-session effect support — fail soft
      // so the player still works, just capped at 100%.
      promise.reject("audio_boost_unavailable", e.message, e)
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
  }
}
