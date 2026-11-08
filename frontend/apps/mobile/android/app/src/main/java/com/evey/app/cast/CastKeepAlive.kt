package com.evey.app.cast

import android.content.Context
import android.net.wifi.WifiManager
import android.os.PowerManager
import android.util.Log
import com.google.android.gms.cast.framework.CastContext
import com.google.android.gms.cast.framework.CastSession
import com.google.android.gms.cast.framework.SessionManagerListener

/**
 * Port of the mechanism a widespread Android Cast client uses, applied to
 * Evey's react-native-google-cast stack.
 *
 * The MediaNotificationService foreground service keeps the process from
 * being frozen or killed, but a foreground service does NOT hold the CPU.
 * Screen off without a wakelock: the CPU suspends, the CASTV2 heartbeat PONGs
 * stop going out, the box closes the sender socket, then tears the receiver
 * down once its last sender is gone. Measured in prod over two runs: the phone
 * goes silent 5 s after loading, the TV dies 3 min 35 and 3 min 45 later. That
 * client holds a PARTIAL_WAKE_LOCK for the whole remote playback; RNGC 4.9.1
 * holds none (zero occurrences in the package).
 *
 * No JS bridge: the listener hooks into the SDK's SessionManager, so acquire
 * and release follow the Cast session itself. A bridge would rely on JS to
 * release - and JS is precisely what dies.
 *
 * Install: CastKeepAlive.install(this) in MainApplication.onCreate(), after
 * super.onCreate() (CastContext requires the main thread).
 * Manifest permissions: WAKE_LOCK, ACCESS_WIFI_STATE.
 */
object CastKeepAlive : SessionManagerListener<CastSession> {

    private const val TAG = "CastKeepAlive"

    // Same safety net as that client: 4 h max. Not reference-counted, so an
    // acquire() while the lock is already held only extends the deadline.
    private const val TIMEOUT_MS = 4L * 60L * 60L * 1000L

    private var wakeLock: PowerManager.WakeLock? = null
    private var wifiLock: WifiManager.WifiLock? = null

    fun install(context: Context) {
        val app = context.applicationContext
        try {
            val pm = app.getSystemService(Context.POWER_SERVICE) as PowerManager
            wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "evey:cast")
                .apply { setReferenceCounted(false) }

            // HIGH_PERF rather than LOW_LATENCY, on purpose: LOW_LATENCY is
            // only active while the app is in the foreground AND the screen
            // is on - exactly the opposite of the case being fixed. It would
            // do nothing where it is needed. HIGH_PERF is deprecated but
            // remains the only mode that holds with the screen off. Either
            // way the wakelock is the load-bearing piece (that client has
            // nothing else); this Wi-Fi lock is an extra belt, best-effort.
            @Suppress("DEPRECATION")
            val mode = WifiManager.WIFI_MODE_FULL_HIGH_PERF
            val wm = app.getSystemService(Context.WIFI_SERVICE) as WifiManager
            wifiLock = wm.createWifiLock(mode, "evey:cast")
                .apply { setReferenceCounted(false) }

            CastContext.getSharedInstance(app).sessionManager
                .addSessionManagerListener(this, CastSession::class.java)
            Log.i(TAG, "installed")
        } catch (e: Exception) {
            // No Play Services / Cast init impossible: no casting, so no lock
            // to hold. Startup is not broken over this.
            Log.w(TAG, "install failed", e)
        }
    }

    private fun acquire() {
        try {
            wakeLock?.acquire(TIMEOUT_MS)
            wifiLock?.let { if (!it.isHeld) it.acquire() }
            Log.i(TAG, "locks acquired")
        } catch (e: Exception) {
            Log.w(TAG, "acquire failed", e)
        }
    }

    private fun release() {
        try {
            wakeLock?.let { if (it.isHeld) it.release() }
            wifiLock?.let { if (it.isHeld) it.release() }
            Log.i(TAG, "locks released")
        } catch (e: Exception) {
            Log.w(TAG, "release failed", e)
        }
    }

    override fun onSessionStarted(session: CastSession, sessionId: String) = acquire()
    override fun onSessionResumed(session: CastSession, wasSuspended: Boolean) = acquire()

    // Suspended = transient network loss: KEEP the CPU so the SDK's
    // reconnection can succeed. The 4 h timeout bounds the worst case.
    override fun onSessionSuspended(session: CastSession, reason: Int) {}

    override fun onSessionEnded(session: CastSession, error: Int) = release()
    override fun onSessionStartFailed(session: CastSession, error: Int) = release()
    override fun onSessionResumeFailed(session: CastSession, error: Int) = release()

    override fun onSessionStarting(session: CastSession) {}
    override fun onSessionResuming(session: CastSession, sessionId: String) {}
    override fun onSessionEnding(session: CastSession) {}
}
