package com.evey.casttv

import android.content.Context
import com.google.android.gms.cast.tv.ReceiverOptionsProvider
import com.google.android.gms.cast.tv.CastReceiverOptions

/**
 * Named by the manifest meta-data the withCastConnect plugin writes. Its only
 * job is to hand the SDK our LaunchRequestChecker, so a cast aimed at another
 * Evey instance (or at a signed-out television) is refused here and falls
 * back to the Web Receiver instead of opening a dead screen.
 */
class EveyCastReceiverOptionsProvider : ReceiverOptionsProvider {
  override fun getOptions(context: Context): CastReceiverOptions {
    return CastReceiverOptions.Builder(context)
      .setLaunchRequestChecker(EveyLaunchRequestChecker(context.applicationContext))
      .setStatusText("Evey")
      .build()
  }
}
