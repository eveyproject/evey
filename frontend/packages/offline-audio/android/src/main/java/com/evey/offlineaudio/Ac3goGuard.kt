package com.evey.offlineaudio

import android.app.ActivityManager
import android.app.ApplicationExitInfo
import android.content.Context
import android.content.IntentFilter
import android.content.SharedPreferences
import android.media.AudioManager
import android.os.Build
import android.os.SystemClock
import android.util.Log

/**
 * Whether this device may declare ac3go to the server. ac3go decodes on the
 * CPU, and a box too slow for it does not fail cleanly: the audio clock falls
 * behind, the picture crawls, then the app stops (2026-10-01, a Chromecast HD).
 * The server's own fallback - video copied, audio transcoded to AAC - is what
 * such a box played fine before ac3go, so every doubt here routes to it:
 *
 * - a launch benchmark finds ac3go too slow on this CPU;
 * - the player's watchdog saw it fall behind (this process only);
 * - the app died while ac3go was decoding: suspect for SUSPECT_MS, or until
 *   another app version.
 */
object Ac3goGuard {
  private const val TAG = "EveyAc3go"
  private const val PREFS = "evey_ac3go_guard"
  private const val SUSPECT_MS = 7L * 24 * 3600 * 1000
  /** Bench median decode time per second of audio above which ac3go is not declared. */
  const val BENCH_SLOW_RTF = 0.35

  @Volatile private var app: Context? = null
  @Volatile private var version = ""
  @Volatile private var playingMarked = false
  @Volatile private var mediaId = ""
  @Volatile private var disabledReason: String? = null
  @Volatile private var bench: Map<String, Any>? = null
  @Volatile private var exitReport: Map<String, Any>? = null

  @Synchronized
  fun init(context: Context) {
    if (app != null) return
    val ctx = context.applicationContext
    version = runCatching {
      ctx.packageManager.getPackageInfo(ctx.packageName, 0).versionName ?: ""
    }.getOrDefault("")
    app = ctx
    // Read before anything of this process can mark itself playing.
    exitReport = readAbnormalExit(ctx)
  }

  private fun prefs(): SharedPreferences? = app?.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

  /** The player's media id, written into the marker so a report names the title. */
  fun setMedia(id: String) {
    mediaId = id
  }

  /**
   * ac3go is about to run (`kind` = playback or bench). Cheap after the first
   * call: decode() calls it per access unit.
   */
  fun markRunning(kind: String) {
    if (playingMarked) return
    playingMarked = true
    prefs()?.edit()
      ?.putString("running", kind)
      ?.putString("media", mediaId)
      ?.putLong("since", System.currentTimeMillis())
      ?.putString("version", version)
      ?.commit()
  }

  /** ac3go stopped cleanly, or the app left the screen (the OS may then kill it). */
  fun markStopped() {
    if (!playingMarked) return
    playingMarked = false
    prefs()?.edit()?.remove("running")?.apply()
  }

  /** Not declared any more for the rest of this process (the watchdog saw it fall behind). */
  fun disable(reason: String) {
    disabledReason = reason
    markSuspect(reason)
  }

  private fun markSuspect(reason: String) {
    Log.w(TAG, "ac3go suspect on this device: $reason")
    prefs()?.edit()
      ?.putLong("suspectUntil", System.currentTimeMillis() + SUSPECT_MS)
      ?.putString("suspectVersion", version)
      ?.putString("suspectReason", reason)
      ?.apply()
  }

  /** Why ac3go is not declared, null when it is. */
  fun blockedBy(): String? {
    disabledReason?.let { return "watchdog:$it" }
    if (bench?.get("slow") == true) return "bench"
    val p = prefs() ?: return null
    val until = p.getLong("suspectUntil", 0)
    if (until > System.currentTimeMillis() && p.getString("suspectVersion", "") == version) {
      return "suspect:${p.getString("suspectReason", "")}"
    }
    return null
  }

  fun playbackDecoders(): List<String> = if (blockedBy() == null) listOf("ac3", "eac3") else emptyList()

  /** The run of a previous process that ended while ac3go was decoding, null if none. */
  fun abnormalExit(): Map<String, Any>? = exitReport

  /**
   * Runs the benchmark once per process (later calls return its result).
   * Marked running while it decodes: a native crash inside it is a verdict too.
   */
  @Synchronized
  fun benchOnce(context: Context): Map<String, Any> {
    bench?.let { return it }
    init(context)
    markRunning("bench")
    val result = try {
      Ac3goBench.run(context, outputMaxChannels(context))
    } catch (t: Throwable) {
      Log.w(TAG, "ac3go bench failed", t)
      mapOf("error" to (t.message ?: t.javaClass.simpleName), "slow" to false)
    } finally {
      markStopped()
    }
    Log.i(TAG, "ac3go bench $result")
    bench = result
    return result
  }

  private fun readAbnormalExit(ctx: Context): Map<String, Any>? {
    val p = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    val kind = p.getString("running", null) ?: return null
    val since = p.getLong("since", 0)
    val report = mutableMapOf<String, Any>(
      "kind" to kind,
      "media" to (p.getString("media", "") ?: ""),
      "since" to since,
      "version" to (p.getString("version", "") ?: ""),
    )
    var exitReason = -1
    var recorded = false
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
      runCatching {
        val am = ctx.getSystemService(ActivityManager::class.java)
        am.getHistoricalProcessExitReasons(ctx.packageName, 0, 5)
          .firstOrNull { it.timestamp >= since }
      }.getOrNull()?.let { info ->
        recorded = true
        exitReason = info.reason
        report["reason"] = reasonName(info.reason)
        report["status"] = info.status
        report["importance"] = info.importance
        report["description"] = info.description ?: ""
        report["pssKb"] = info.pss
        report["rssKb"] = info.rss
        report["at"] = info.timestamp
      }
    }
    p.edit().remove("running").apply()
    // A user closing the app, or the app ending itself, says nothing about
    // ac3go; a crash, a hang or memory pressure while it decoded does. Android
    // 11+ records every process death, so none recorded is a power cut or a
    // reboot; before 11 nothing tells them apart from a crash.
    val innocent = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) !recorded || exitReason in INNOCENT_EXITS else false
    if (!recorded && Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) report["reason"] = "none_recorded"
    report["suspect"] = !innocent
    Log.w(TAG, "previous run ended while ac3go was running: $report")
    if (!innocent) {
      val until = System.currentTimeMillis() + SUSPECT_MS
      p.edit()
        .putLong("suspectUntil", until)
        .putString("suspectVersion", version)
        .putString("suspectReason", "exit:${report["reason"] ?: "unknown"}")
        .apply()
    }
    return report
  }

  private val INNOCENT_EXITS = setOf(
    ApplicationExitInfo.REASON_USER_REQUESTED,
    ApplicationExitInfo.REASON_USER_STOPPED,
    ApplicationExitInfo.REASON_EXIT_SELF,
    ApplicationExitInfo.REASON_PACKAGE_UPDATED,
    ApplicationExitInfo.REASON_DEPENDENCY_DIED,
  )

  private fun reasonName(reason: Int): String = when (reason) {
    ApplicationExitInfo.REASON_ANR -> "anr"
    ApplicationExitInfo.REASON_CRASH -> "crash"
    ApplicationExitInfo.REASON_CRASH_NATIVE -> "crash_native"
    ApplicationExitInfo.REASON_LOW_MEMORY -> "low_memory"
    ApplicationExitInfo.REASON_SIGNALED -> "signaled"
    ApplicationExitInfo.REASON_EXCESSIVE_RESOURCE_USAGE -> "excessive_resource"
    ApplicationExitInfo.REASON_FREEZER -> "freezer"
    ApplicationExitInfo.REASON_INITIALIZATION_FAILURE -> "init_failure"
    ApplicationExitInfo.REASON_PERMISSION_CHANGE -> "permission_change"
    ApplicationExitInfo.REASON_OTHER -> "other"
    ApplicationExitInfo.REASON_USER_REQUESTED -> "user_requested"
    ApplicationExitInfo.REASON_USER_STOPPED -> "user_stopped"
    ApplicationExitInfo.REASON_EXIT_SELF -> "exit_self"
    ApplicationExitInfo.REASON_PACKAGE_UPDATED -> "package_updated"
    ApplicationExitInfo.REASON_DEPENDENCY_DIED -> "dependency_died"
    else -> "code_$reason"
  }

  // A plugged HDMI sink states its channel count (the same evidence the audio
  // probe reports); any other output, or none stated, gets ac3go's stereo.
  fun outputMaxChannels(context: Context): Int {
    val hdmi = runCatching {
      context.registerReceiver(null, IntentFilter(AudioManager.ACTION_HDMI_AUDIO_PLUG))
    }.getOrNull() ?: return 2
    if (hdmi.getIntExtra(AudioManager.EXTRA_AUDIO_PLUG_STATE, 0) != 1) return 2
    return hdmi.getIntExtra(AudioManager.EXTRA_MAX_CHANNEL_COUNT, 2).coerceIn(2, 8)
  }
}

/**
 * Decodes a bundled second of E-AC-3 5.1 at 768 kb/s (pink noise: every band
 * coded, the dearest case) with the output the player would use, and measures
 * decode time per second of audio.
 */
object Ac3goBench {
  private const val ASSET = "ac3go_bench.eac3"
  private const val PASSES = 4
  private const val SAMPLES_PER_FRAME = 1536
  private const val RATE = 48_000

  fun run(context: Context, maxChannels: Int): Map<String, Any> {
    val clip = context.assets.open(ASSET).use { it.readBytes() }
    val units = splitFrames(clip)
    require(units.isNotEmpty()) { "bench clip holds no syncframe" }
    val audioSec = units.size.toDouble() * SAMPLES_PER_FRAME / RATE
    val go = Offlineaudio.newFrameDecoder(maxChannels.toLong())
    val rtfs = DoubleArray(PASSES)
    val started = SystemClock.elapsedRealtime()
    for (pass in 0 until PASSES) {
      go.reset()
      val t0 = System.nanoTime()
      for (u in units) go.decode(u)
      rtfs[pass] = (System.nanoTime() - t0) / 1e9 / audioSec
    }
    // The first pass pays the cold start (page faults, caches): reported, not judged.
    val judged = rtfs.copyOfRange(1, PASSES).sorted()
    val median = judged[judged.size / 2]
    return mapOf(
      "rtf" to median,
      "rtfCold" to rtfs[0],
      "rtfMax" to judged.last(),
      "maxChannels" to maxChannels,
      "frames" to units.size,
      "bad" to go.badFrames(),
      "wallMs" to (SystemClock.elapsedRealtime() - started),
      "slow" to (median > Ac3goGuard.BENCH_SLOW_RTF),
    )
  }

  /** E-AC-3 syncframes: 0x0B77, then frmsiz (11 bits) + 1 words. */
  fun splitFrames(b: ByteArray): List<ByteArray> {
    val out = ArrayList<ByteArray>()
    var i = 0
    while (i + 4 <= b.size) {
      if (b[i] != 0x0B.toByte() || b[i + 1] != 0x77.toByte()) break
      val size = (((b[i + 2].toInt() and 0x07) shl 8) or (b[i + 3].toInt() and 0xFF)) + 1
      val end = i + size * 2
      if (end > b.size) break
      out.add(b.copyOfRange(i, end))
      i = end
    }
    return out
  }
}
