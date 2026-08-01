package com.evey.offlineaudio

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder

/**
 * Holds the process in the foreground while the audio pass runs. The service
 * does no work itself - the pass thread lives in the module - it exists so
 * the OEM freezer (ColorOS and friends) does not suspend the process the
 * moment the app leaves the screen, which turned a minutes-long pass into
 * hours of frozen nothing on the first field run.
 */
class OfflineAudioService : Service() {

  override fun onBind(intent: Intent?): IBinder? = null

  override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
    val label = intent?.getStringExtra(EXTRA_LABEL) ?: ""
    val notification = buildNotification(this, label, 0)
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
      startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
    } else {
      startForeground(NOTIFICATION_ID, notification)
    }
    // Restarting with a null intent after a kill would resurrect a service
    // with no pass to guard; the startup sweep re-runs the pass instead.
    return START_NOT_STICKY
  }

  companion object {
    const val NOTIFICATION_ID = 0xE7E1
    private const val CHANNEL_ID = "evey-offline-audio"
    private const val EXTRA_LABEL = "label"

    fun start(context: Context, label: String) {
      val intent = Intent(context, OfflineAudioService::class.java).putExtra(EXTRA_LABEL, label)
      // On Android 12+ this throws if the app is in a background state that
      // may not start foreground services; the caller degrades gracefully
      // (the pass still runs, just without freeze protection).
      context.startForegroundService(intent)
    }

    fun stop(context: Context) {
      context.stopService(Intent(context, OfflineAudioService::class.java))
    }

    fun updateProgress(context: Context, label: String, permille: Long) {
      val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
      manager.notify(NOTIFICATION_ID, buildNotification(context, label, permille))
    }

    private fun buildNotification(context: Context, label: String, permille: Long): Notification {
      ensureChannel(context)
      val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
        Notification.Builder(context, CHANNEL_ID)
      } else {
        @Suppress("DEPRECATION")
        Notification.Builder(context)
      }
      return builder
        .setContentTitle("Optimisation du son")
        .setContentText(label)
        .setSmallIcon(android.R.drawable.stat_sys_download)
        .setOngoing(true)
        .setOnlyAlertOnce(true)
        .setProgress(1000, permille.toInt(), permille <= 0)
        .build()
    }

    private fun ensureChannel(context: Context) {
      if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
      val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
      if (manager.getNotificationChannel(CHANNEL_ID) != null) return
      manager.createNotificationChannel(
        NotificationChannel(CHANNEL_ID, "Optimisation du son", NotificationManager.IMPORTANCE_LOW),
      )
    }
  }
}
