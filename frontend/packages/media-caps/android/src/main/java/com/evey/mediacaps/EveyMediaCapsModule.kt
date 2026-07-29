package com.evey.mediacaps

import android.content.Context
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.media.AudioAttributes
import android.media.AudioDeviceInfo
import android.media.AudioFormat
import android.media.AudioManager
import android.media.AudioTrack
import android.media.MediaCodecList
import android.os.Build
import android.provider.Settings
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * Raw audio-output capability evidence for THIS device.
 *
 * Why not just MediaCodecList (what the mobile app shipped first): that answers
 * "do I have a DECODER", which is the right question on a phone driving its own
 * speakers and the WRONG one on a TV box. The dominant TV case is HDMI
 * passthrough, where the box decodes nothing and hands the AC-3/E-AC-3 bitstream
 * to a TV or AV receiver that does. Declaring from decoders alone makes us
 * transcode audio for a box that would have passed it through untouched.
 *
 * So this module reports the four independent sources SEPARATELY and does not
 * collapse them into a verdict. Interpretation belongs on the JS side, on
 * evidence gathered from real devices, not on a mapping guessed here. Values
 * that are platform integers (encodings, device types, the surround setting)
 * are reported RAW for the same reason: the beacon exists to correct our
 * assumptions, and a mapping applied before the data is seen would bake them in.
 */
class EveyMediaCapsModule : Module() {

  override fun definition() = ModuleDefinition {
    Name("EveyMediaCaps")

    AsyncFunction("getAudioProbe") {
      val context: Context = appContext.reactContext
        ?: throw IllegalStateException("no react context")
      probe(context)
    }
  }

  private fun probe(context: Context): Map<String, Any?> {
    val audio = context.getSystemService(Context.AUDIO_SERVICE) as? AudioManager
    return mapOf(
      "apiLevel" to Build.VERSION.SDK_INT,
      "isTv" to isTv(context),
      "deviceModel" to "${Build.MANUFACTURER} ${Build.MODEL}",
      "decoders" to decoders(),
      "directPlayback" to directPlayback(),
      "directProfiles" to directProfiles(audio),
      "hdmi" to hdmi(context),
      "surroundOutputRaw" to surroundOutputRaw(context),
      "outputDeviceTypes" to outputDeviceTypes(audio),
    )
  }

  private fun isTv(context: Context): Boolean =
    context.packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)

  /** evey/ffprobe codec name -> Android MIME, for the decoder half. */
  private val mimeByCodec = linkedMapOf(
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

  private fun decoders(): List<String> {
    val types = MediaCodecList(MediaCodecList.REGULAR_CODECS)
      .codecInfos
      .filter { !it.isEncoder }
      .flatMap { it.supportedTypes.asList() }
      .map { it.lowercase() }
      .toSet()
    return mimeByCodec.filterValues { types.contains(it) }.keys.toList()
  }

  /**
   * Can the CURRENT audio sink take this compressed format directly - which is
   * exactly "does the TV or the receiver declare itself capable". API 29+ only;
   * below that the HDMI encodings list is the only signal, hence reporting both.
   */
  @Suppress("DEPRECATION")
  private fun directPlayback(): Map<String, Boolean> {
    // Deprecated since API 34 in favour of getDirectProfilesForAttributes
    // (reported separately below), but it is the ONLY direct-playback answer
    // available on API 29..32, which is most of the Android TV boxes in the
    // field. Kept for that range, and as a cross-check above it.
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return emptyMap()
    val attrs = AudioAttributes.Builder()
      .setUsage(AudioAttributes.USAGE_MEDIA)
      .setContentType(AudioAttributes.CONTENT_TYPE_MOVIE)
      .build()
    val candidates = linkedMapOf(
      "ac3" to AudioFormat.ENCODING_AC3,
      "eac3" to AudioFormat.ENCODING_E_AC3,
      "eac3_joc" to AudioFormat.ENCODING_E_AC3_JOC,
      "dts" to AudioFormat.ENCODING_DTS,
      "dts_hd" to AudioFormat.ENCODING_DTS_HD,
      "truehd" to AudioFormat.ENCODING_DOLBY_TRUEHD,
      "ac4" to AudioFormat.ENCODING_AC4,
    )
    return candidates.mapValues { (_, encoding) ->
      val format = AudioFormat.Builder()
        .setEncoding(encoding)
        .setSampleRate(48_000)
        .setChannelMask(AudioFormat.CHANNEL_OUT_5POINT1)
        .build()
      runCatching { AudioTrack.isDirectPlaybackSupported(format, attrs) }.getOrDefault(false)
    }
  }

  /**
   * The modern replacement for isDirectPlaybackSupported (API 33+): the audio
   * profiles the sink accepts without decoding, each with its channel masks.
   * Richer than the boolean map - it also says HOW MANY channels the sink takes
   * for a given format, which is what decides 5.1 versus a stereo downmix.
   * Encodings stay raw AudioFormat.ENCODING_* values, same rationale as HDMI.
   */
  private fun directProfiles(audio: AudioManager?): List<Map<String, Any?>> {
    if (audio == null || Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return emptyList()
    val attrs = AudioAttributes.Builder()
      .setUsage(AudioAttributes.USAGE_MEDIA)
      .setContentType(AudioAttributes.CONTENT_TYPE_MOVIE)
      .build()
    return runCatching {
      audio.getDirectProfilesForAttributes(attrs).map { profile ->
        mapOf(
          "encoding" to profile.format,
          "channelMasks" to profile.channelMasks.toList(),
        )
      }
    }.getOrDefault(emptyList())
  }

  /**
   * What the connected HDMI sink advertises. Read from the STICKY broadcast
   * (null receiver = read the last value without subscribing), so it works
   * without a running player. plugState 0 means nothing is plugged in, in which
   * case the encodings list says nothing about the eventual TV.
   */
  private fun hdmi(context: Context): Map<String, Any?> {
    val intent = runCatching {
      context.registerReceiver(null, IntentFilter(AudioManager.ACTION_HDMI_AUDIO_PLUG))
    }.getOrNull() ?: return mapOf("available" to false)
    return mapOf(
      "available" to true,
      "plugState" to intent.getIntExtra(AudioManager.EXTRA_AUDIO_PLUG_STATE, -1),
      "encodings" to (intent.getIntArrayExtra(AudioManager.EXTRA_ENCODINGS)?.toList() ?: emptyList<Int>()),
      "maxChannelCount" to intent.getIntExtra(AudioManager.EXTRA_MAX_CHANNEL_COUNT, -1),
    )
  }

  /**
   * The Android TV "Surround sound" system setting. When the user sets it to
   * never, the platform HONESTLY reports no AC-3 and we transcode - that is
   * correct behaviour, and this value is what distinguishes it from a bug on
   * our side. Read by key: the constant is not public API.
   */
  private fun surroundOutputRaw(context: Context): Int =
    runCatching {
      Settings.Global.getInt(context.contentResolver, "encoded_surround_output", -1)
    }.getOrDefault(-1)

  private fun outputDeviceTypes(audio: AudioManager?): List<Int> {
    if (audio == null) return emptyList()
    return runCatching {
      audio.getDevices(AudioManager.GET_DEVICES_OUTPUTS).map { it.type }
    }.getOrDefault(emptyList())
  }

  @Suppress("unused")
  private fun deviceTypeIsHdmi(type: Int): Boolean =
    type == AudioDeviceInfo.TYPE_HDMI || type == AudioDeviceInfo.TYPE_HDMI_ARC
}
