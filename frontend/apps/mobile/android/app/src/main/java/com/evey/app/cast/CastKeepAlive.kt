package com.evey.app.cast

import android.content.Context
import android.net.wifi.WifiManager
import android.os.PowerManager
import android.util.Log
import com.google.android.gms.cast.framework.CastContext
import com.google.android.gms.cast.framework.CastSession
import com.google.android.gms.cast.framework.SessionManagerListener

/**
 * Transposition du mecanisme d'un client Android Cast tres repandu a la
 * stack react-native-google-cast d'Evey.
 *
 * Le FGS MediaNotificationService (beta.15) empeche le gel et le kill du
 * processus, mais un service de premier plan ne retient PAS le CPU. Ecran
 * eteint sans wakelock : le CPU se suspend, les PONG du heartbeat CASTV2 ne
 * partent plus, la box ferme la socket sender, puis demonte le recepteur quand
 * son dernier sender disparait. Mesure prod du 2026-07-28, deux runs : le
 * telephone se tait 5 s apres le chargement, la TV meurt 3 min 35 puis 3 min 45
 * plus tard. Ce client tient un PARTIAL_WAKE_LOCK pendant toute la lecture
 * distante ; RNGC 4.9.1 n'en tient aucun (zero occurrence dans le paquet).
 *
 * Aucun pont JS : le listener est branche sur le SessionManager du SDK, donc
 * l'acquisition et la liberation suivent la session Cast elle-meme. Un pont
 * dependrait du JS pour relacher - or c'est precisement le JS qui meurt.
 *
 * Install : CastKeepAlive.install(this) dans MainApplication.onCreate(), apres
 * super.onCreate() (CastContext exige le thread principal).
 * Permissions manifest : WAKE_LOCK, ACCESS_WIFI_STATE.
 */
object CastKeepAlive : SessionManagerListener<CastSession> {

    private const val TAG = "CastKeepAlive"

    // Meme garde-fou que ce client : 4 h max. Non-refcounted, donc un acquire()
    // alors que le verrou est deja tenu ne fait que reprolonger le delai.
    private const val TIMEOUT_MS = 4L * 60L * 60L * 1000L

    private var wakeLock: PowerManager.WakeLock? = null
    private var wifiLock: WifiManager.WifiLock? = null

    fun install(context: Context) {
        val app = context.applicationContext
        try {
            val pm = app.getSystemService(Context.POWER_SERVICE) as PowerManager
            wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "evey:cast")
                .apply { setReferenceCounted(false) }

            // HIGH_PERF et pas LOW_LATENCY, volontairement : le mode
            // LOW_LATENCY n'est actif que si l'app est au premier plan ET
            // l'ecran allume - exactement l'inverse du cas qu'on repare. Il
            // serait sans effet la ou on en a besoin. HIGH_PERF est deprecie
            // mais reste le seul a porter ecran eteint. De toute facon le
            // wakelock est la piece porteuse (ce client n'a que lui) ; ce
            // verrou Wi-Fi est une ceinture en plus, best-effort.
            @Suppress("DEPRECATION")
            val mode = WifiManager.WIFI_MODE_FULL_HIGH_PERF
            val wm = app.getSystemService(Context.WIFI_SERVICE) as WifiManager
            wifiLock = wm.createWifiLock(mode, "evey:cast")
                .apply { setReferenceCounted(false) }

            CastContext.getSharedInstance(app).sessionManager
                .addSessionManagerListener(this, CastSession::class.java)
            Log.i(TAG, "installed")
        } catch (e: Exception) {
            // Pas de Play Services / init cast impossible : pas de cast, donc
            // pas de verrou a tenir. On ne casse pas le demarrage pour ca.
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

    // Suspension = coupure reseau transitoire : on GARDE le CPU pour que la
    // reconnexion du SDK puisse aboutir. Le delai de 4 h borne le pire cas.
    override fun onSessionSuspended(session: CastSession, reason: Int) {}

    override fun onSessionEnded(session: CastSession, error: Int) = release()
    override fun onSessionStartFailed(session: CastSession, error: Int) = release()
    override fun onSessionResumeFailed(session: CastSession, error: Int) = release()

    override fun onSessionStarting(session: CastSession) {}
    override fun onSessionResuming(session: CastSession, sessionId: String) {}
    override fun onSessionEnding(session: CastSession) {}
}
