package com.evey.app

import android.hardware.display.DisplayManager
import android.media.MediaCodecList
import android.view.Display
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod

/**
 * MediaCaps - real decoder capabilities of THIS device, so the play-session
 * declares to the server only what the hardware can actually decode.
 *
 * The static codec list the app used to send included ac3/eac3, but plenty of
 * devices ship without Dolby decoders (this very phone has none): the server
 * then DIRECT_PLAYs an eac3 track and the video plays silent. MediaCodecList is
 * the source of truth ExoPlayer itself consults, so declaring from it keeps
 * direct-play for everything the device honestly decodes and lets the server
 * transcode the audio (video stays copy) for the rest.
 */
class MediaCapsModule(reactContext: ReactApplicationContext) :
  ReactContextBaseJavaModule(reactContext) {

  override fun getName() = "MediaCaps"

  @ReactMethod
  fun getSupportedAudioCodecs(promise: Promise) {
    try {
      // evey/ffprobe codec name -> Android MIME type.
      val mimeByCodec = linkedMapOf(
        "aac" to "audio/mp4a-latm",
        "mp3" to "audio/mpeg",
        "opus" to "audio/opus",
        "flac" to "audio/flac",
        "vorbis" to "audio/vorbis",
        "ac3" to "audio/ac3",
        "eac3" to "audio/eac3",
        "dts" to "audio/vnd.dts",
        "truehd" to "audio/true-hd",
      )
      val decoders = MediaCodecList(MediaCodecList.REGULAR_CODECS)
        .codecInfos
        .filter { !it.isEncoder }
        .flatMap { it.supportedTypes.asList() }
        .map { it.lowercase() }
        .toSet()
      val supported = Arguments.createArray()
      for ((codec, mime) in mimeByCodec) {
        if (decoders.contains(mime)) supported.pushString(codec)
      }
      promise.resolve(supported)
    } catch (e: Exception) {
      // Fail soft: JS falls back to its conservative baseline list.
      promise.reject("media_caps_unavailable", e.message, e)
    }
  }

  /**
   * HDR formats the default display can render, as evey names: hdr10, hlg,
   * hdr10plus, dovi. Empty = SDR panel. Lets the play-session declare
   * supports_hdr so an HDR10 HEVC source video-COPIES (on-demand path, instant
   * seek, exact colors) instead of being tonemapped in software.
   */
  @ReactMethod
  fun getHdrTypes(promise: Promise) {
    try {
      val dm = reactApplicationContext.getSystemService(DisplayManager::class.java)
      val display = dm?.getDisplay(Display.DEFAULT_DISPLAY)
      val types = Arguments.createArray()
      display?.hdrCapabilities?.supportedHdrTypes?.forEach { t ->
        when (t) {
          Display.HdrCapabilities.HDR_TYPE_HDR10 -> types.pushString("hdr10")
          Display.HdrCapabilities.HDR_TYPE_HLG -> types.pushString("hlg")
          Display.HdrCapabilities.HDR_TYPE_HDR10_PLUS -> types.pushString("hdr10plus")
          Display.HdrCapabilities.HDR_TYPE_DOLBY_VISION -> types.pushString("dovi")
        }
      }
      promise.resolve(types)
    } catch (e: Exception) {
      promise.reject("media_caps_unavailable", e.message, e)
    }
  }
}
