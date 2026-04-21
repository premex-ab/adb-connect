package se.premex.adbgate.adb

import android.content.Context
import android.provider.Settings

object AdbWifi {
    private const val KEY = "adb_wifi_enabled"

    fun isEnabled(context: Context): Boolean =
        Settings.Global.getInt(context.contentResolver, KEY, 0) == 1

    fun setEnabled(context: Context, enabled: Boolean): Boolean =
        Settings.Global.putInt(context.contentResolver, KEY, if (enabled) 1 else 0)
}
