package se.premex.adbgate.service

import android.content.Context
import android.content.Intent
import android.os.Build

object ServiceController {
    fun start(context: Context) {
        val intent = Intent(context, AdbGateService::class.java).setAction(AdbGateService.ACTION_START)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) context.startForegroundService(intent)
        else context.startService(intent)
    }
    fun stop(context: Context) {
        val intent = Intent(context, AdbGateService::class.java).setAction(AdbGateService.ACTION_STOP)
        context.startService(intent)
    }
}
