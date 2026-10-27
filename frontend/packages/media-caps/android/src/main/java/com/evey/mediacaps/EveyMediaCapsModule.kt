package com.evey.mediacaps

import android.app.ActivityManager
import android.content.Context
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.media.AudioAttributes
import android.media.AudioDeviceInfo
import android.media.AudioFormat
import android.media.AudioManager
import android.media.AudioTrack
import android.media.MediaCodecInfo
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

  private val UI_PREFS = "evey_ui"
  private val UI_SCALE_KEY = "scale"
  private val LITE_MODE_KEY = "lite_mode"

  private fun uiPrefs() =
    appContext.reactContext?.getSharedPreferences(UI_PREFS, Context.MODE_PRIVATE)

  private fun restartApp() {
    val context = appContext.reactContext ?: return
    val launch = context.packageManager.getLaunchIntentForPackage(context.packageName) ?: return
    launch.addFlags(
      android.content.Intent.FLAG_ACTIVITY_NEW_TASK or
        android.content.Intent.FLAG_ACTIVITY_CLEAR_TASK or
        android.content.Intent.FLAG_ACTIVITY_CLEAR_TOP,
    )
    appContext.currentActivity?.finishAffinity()
    context.startActivity(launch)
    Runtime.getRuntime().exit(0)
  }

  override fun definition() = ModuleDefinition {
    Name("EveyMediaCaps")

    AsyncFunction("getAudioProbe") {
      val context: Context = appContext.reactContext
        ?: throw IllegalStateException("no react context")
      probe(context)
    }

    // Public Downloads collection root, or null when this platform can't offer
    // one the app may write to without a permission dance. Android 11+ routes
    // raw file writes into standard collections through FUSE and keeps the app
    // owner of its own files (delete included); below that, public-dir writes
    // need the legacy storage permission, so callers fall back to the private
    // dir. Used so downloaded media lands where VLC and file managers can see
    // it - owner decision: the in-app parental gate must not dictate storage.
    Function("getPublicDownloadsDir") {
      if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) return@Function null
      @Suppress("DEPRECATION")
      val dir = android.os.Environment.getExternalStoragePublicDirectory(
        android.os.Environment.DIRECTORY_DOWNLOADS,
      )
      dir?.absolutePath
    }

    // Raw java.io.File operations, for the files expo-file-system refuses to
    // touch: its legacy API is scoped to the app's own directories and
    // REJECTS anything under the public Downloads collection - even files
    // this app created and owns (observed in the field: the audio pass
    // finished its 1.7 GB rewrite and then could not swap it into place).
    // The plain File API goes through FUSE with the app's ownership and works
    // on both private and public paths.
    Function("deleteFile") { path: String ->
      val f = java.io.File(path)
      f.delete() || !f.exists()
    }

    // Same-volume rename; returns false rather than throwing so the caller
    // can fall back or surface its own error.
    Function("moveFile") { from: String, to: String ->
      java.io.File(from).renameTo(java.io.File(to))
    }

    // Highest H.264 level_idc (41 = 4.1, 51 = 5.1) a HARDWARE decoder on this
    // device takes; software decoders only count when no hardware one exists.
    // 0 = unknown, the caller then declares nothing (level-blind, as before).
    // UI scale (TV): the percentage the app's density is scaled by at process
    // start (MainApplication.applyUiScale, see the tv app's withUiScale
    // plugin). Stored here, read there: the two halves share the preference
    // file and key. null = never set (the platform applies its own default).
    Function("getUiScalePercent") {
      val prefs = uiPrefs() ?: return@Function null
      if (prefs.contains(UI_SCALE_KEY)) prefs.getInt(UI_SCALE_KEY, 100) else null
    }

    // Persists the scale and restarts the app: React Native reads the density
    // once per process, so a new value only shows after a cold start. The
    // relaunch intent is handed to the system before the process exits; the
    // activity manager brings the app back on it.
    Function("setUiScalePercent") { percent: Int ->
      val prefs = uiPrefs() ?: return@Function false
      prefs.edit().putInt(UI_SCALE_KEY, percent.coerceIn(50, 150)).commit()
      restartApp()
      true
    }

    Function("getMaxH264Level") {
      try { maxH264Level() } catch (e: Exception) { 0 }
    }

    // The video decoders' hard limits: the largest picture they open and the
    // codecs they take at 10 bits. A decoder listed as "hevc" may refuse 4K or
    // Main10; declared by name alone, the server direct-played such streams
    // into a decoder error.
    Function("getVideoDecoderLimits") {
      try { videoDecoderLimits() } catch (e: Exception) { null }
    }

    // What the light mode (TV) decides from: the memory this device has, and
    // the viewer's choice from Settings ("auto", "on", "off"), kept in the same
    // preference file as the UI scale. The verdict is the JS side's.
    Function("getDeviceProfile") {
      val context = appContext.reactContext ?: return@Function null
      val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
      val info = ActivityManager.MemoryInfo().also { am.getMemoryInfo(it) }
      mapOf(
        "lowRam" to am.isLowRamDevice,
        "totalMb" to (info.totalMem / (1024 * 1024)).toInt(),
        "memoryClassMb" to am.memoryClass,
        "liteMode" to (uiPrefs()?.getString(LITE_MODE_KEY, null) ?: "auto"),
      )
    }

    Function("setLiteMode") { mode: String ->
      val prefs = uiPrefs() ?: return@Function false
      prefs.edit().putString(LITE_MODE_KEY, mode).commit()
    }

    // HDR formats the DISPLAY currently attached can render, as evey names.
    // Empty = SDR panel, and the server keeps its tone-mapping.
    //
    // Same shape as the audio half of this module, and for the same reason:
    // this is a RESTITUTION capability, not a decode capability. A box whose
    // HEVC decoder eats Main10 all day long still needs the tonemap when the
    // panel behind the HDMI cable is SDR - measured on an Android TV box driving an
    // old SDR panel (supportedHdrTypes=[] on every mode).
    //
    // Read per call, never memoised: the display changes under a running app
    // (HDMI re-plugged, AVR powered on, mode switch), exactly like the audio
    // output does - see probeMaxOutputChannels' contract on the JS side.
    AsyncFunction("getHdrTypes") {
      val context: Context = appContext.reactContext
        ?: throw IllegalStateException("no react context")
      hdrTypes(context)
    }
  }

  /**
   * Display.HdrCapabilities -> evey names. An unreadable display reports an
   * empty list, which the server reads as SDR: guessing HDR is the expensive
   * mistake (a PQ source encoded without tone-mapping renders washed out).
   */
  private fun hdrTypes(context: Context): List<String> {
    val dm = context.getSystemService(Context.DISPLAY_SERVICE) as? android.hardware.display.DisplayManager
      ?: return emptyList()
    val display = dm.getDisplay(android.view.Display.DEFAULT_DISPLAY) ?: return emptyList()
    @Suppress("DEPRECATION")
    val supported = display.hdrCapabilities?.supportedHdrTypes ?: return emptyList()
    // toList(): supportedHdrTypes is an IntArray, which has no mapNotNull.
    return supported.toList().mapNotNull {
      when (it) {
        android.view.Display.HdrCapabilities.HDR_TYPE_HDR10 -> "hdr10"
        android.view.Display.HdrCapabilities.HDR_TYPE_HLG -> "hlg"
        android.view.Display.HdrCapabilities.HDR_TYPE_HDR10_PLUS -> "hdr10plus"
        android.view.Display.HdrCapabilities.HDR_TYPE_DOLBY_VISION -> "dovi"
        else -> null
      }
    }
  }

  private val avcLevelIdc = mapOf(
    MediaCodecInfo.CodecProfileLevel.AVCLevel1 to 10,
    MediaCodecInfo.CodecProfileLevel.AVCLevel1b to 9,
    MediaCodecInfo.CodecProfileLevel.AVCLevel11 to 11,
    MediaCodecInfo.CodecProfileLevel.AVCLevel12 to 12,
    MediaCodecInfo.CodecProfileLevel.AVCLevel13 to 13,
    MediaCodecInfo.CodecProfileLevel.AVCLevel2 to 20,
    MediaCodecInfo.CodecProfileLevel.AVCLevel21 to 21,
    MediaCodecInfo.CodecProfileLevel.AVCLevel22 to 22,
    MediaCodecInfo.CodecProfileLevel.AVCLevel3 to 30,
    MediaCodecInfo.CodecProfileLevel.AVCLevel31 to 31,
    MediaCodecInfo.CodecProfileLevel.AVCLevel32 to 32,
    MediaCodecInfo.CodecProfileLevel.AVCLevel4 to 40,
    MediaCodecInfo.CodecProfileLevel.AVCLevel41 to 41,
    MediaCodecInfo.CodecProfileLevel.AVCLevel42 to 42,
    MediaCodecInfo.CodecProfileLevel.AVCLevel5 to 50,
    MediaCodecInfo.CodecProfileLevel.AVCLevel51 to 51,
    MediaCodecInfo.CodecProfileLevel.AVCLevel52 to 52,
    MediaCodecInfo.CodecProfileLevel.AVCLevel6 to 60,
    MediaCodecInfo.CodecProfileLevel.AVCLevel61 to 61,
    MediaCodecInfo.CodecProfileLevel.AVCLevel62 to 62,
  )

  private val videoMimes = linkedMapOf(
    "h264" to "video/avc",
    "hevc" to "video/hevc",
    "vp9" to "video/x-vnd.on2.vp9",
    "av1" to "video/av01",
  )

  // The profiles that mean "10-bit 4:2:0" per codec (HDR ones included: a
  // decoder that takes Main10 HDR10 takes Main10).
  private val tenBitProfiles = mapOf(
    "hevc" to setOf(
      MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10,
      MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10HDR10,
      MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10HDR10Plus,
    ),
    "vp9" to setOf(
      MediaCodecInfo.CodecProfileLevel.VP9Profile2,
      MediaCodecInfo.CodecProfileLevel.VP9Profile2HDR,
      MediaCodecInfo.CodecProfileLevel.VP9Profile2HDR10Plus,
    ),
    "av1" to setOf(
      MediaCodecInfo.CodecProfileLevel.AV1ProfileMain10,
      MediaCodecInfo.CodecProfileLevel.AV1ProfileMain10HDR10,
      MediaCodecInfo.CodecProfileLevel.AV1ProfileMain10HDR10Plus,
    ),
  )

  // Same pool rule as maxH264Level: the hardware decoders when there are
  // any, else whatever decodes the type. Per codec: the largest frame any
  // decoder of the pool opens; overall: the largest across codecs.
  private fun videoDecoderLimits(): Map<String, Any?> {
    val infos = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.filter { !it.isEncoder }
    var maxWidth = 0
    var maxHeight = 0
    val tenBit = mutableListOf<String>()
    val perCodec = linkedMapOf<String, Map<String, Int>>()
    for ((codec, mime) in videoMimes) {
      val all = infos.filter { it.supportedTypes.any { t -> t.equals(mime, ignoreCase = true) } }
      if (all.isEmpty()) continue
      val hardware = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) all.filter { it.isHardwareAccelerated } else emptyList()
      val pool = if (hardware.isNotEmpty()) hardware else all
      var w = 0
      var h = 0
      var ten = false
      for (info in pool) {
        val caps = runCatching { info.getCapabilitiesForType(mime) }.getOrNull() ?: continue
        caps.videoCapabilities?.let { v ->
          w = maxOf(w, v.supportedWidths.upper)
          h = maxOf(h, v.supportedHeights.upper)
        }
        val profiles = tenBitProfiles[codec]
        if (profiles != null && caps.profileLevels.any { profiles.contains(it.profile) }) ten = true
      }
      perCodec[codec] = mapOf("maxWidth" to w, "maxHeight" to h)
      if (ten) tenBit += codec
      maxWidth = maxOf(maxWidth, w)
      maxHeight = maxOf(maxHeight, h)
    }
    return mapOf(
      "maxWidth" to maxWidth,
      "maxHeight" to maxHeight,
      "tenBitCodecs" to tenBit,
      "codecs" to perCodec,
    )
  }

  // A phone whose hardware stops at 4.1 would drop a 5.1 stream onto its
  // software decoder and stutter: the hardware ceiling is the honest one.
  private fun maxH264Level(): Int {
    val avc = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos
      .filter { !it.isEncoder && it.supportedTypes.any { t -> t.equals("video/avc", ignoreCase = true) } }
    if (avc.isEmpty()) return 0
    val hardware = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) avc.filter { it.isHardwareAccelerated } else emptyList()
    val pool = if (hardware.isNotEmpty()) hardware else avc
    return pool.maxOf { info ->
      info.getCapabilitiesForType("video/avc").profileLevels.maxOfOrNull { avcLevelIdc[it.level] ?: 0 } ?: 0
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
