package se.premex.adbgate.adb

import android.content.Context
import android.content.Intent
import android.net.wifi.WifiManager
import android.os.Build
import android.provider.Settings
import android.util.Log

object WifiHelper {
    private const val TAG = "AdbGate.WifiHelper"

    fun isEnabled(context: Context): Boolean {
        val wm = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        return wm.isWifiEnabled
    }

    @Suppress("DEPRECATION")
    fun tryEnable(context: Context): Boolean {
        val wm = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        if (wm.isWifiEnabled) return true
        return try {
            val ok = wm.setWifiEnabled(true)
            Log.i(TAG, "WifiManager.setWifiEnabled returned $ok (deprecated since API 29 — typically no-op for non-system apps)")
            ok || wm.isWifiEnabled
        } catch (e: Exception) {
            Log.w(TAG, "WifiManager.setWifiEnabled threw: ${e.message}")
            false
        }
    }

    fun panelIntent(): Intent {
        val intent = if (Build.VERSION.SDK_INT >= 29) Intent(Settings.Panel.ACTION_WIFI)
        else Intent(Settings.ACTION_WIFI_SETTINGS)
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        return intent
    }
}
