package se.premex.adbgate.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import se.premex.adbgate.R
import se.premex.adbgate.adb.AdbWifi
import se.premex.adbgate.adb.PairCodeBridge
import se.premex.adbgate.adb.PairingDiscovery
import se.premex.adbgate.data.EncryptedConfigStore
import se.premex.adbgate.ws.WsClient
import se.premex.adbgate.ws.WsEvents

class AdbGateService : Service(), WsEvents {
    private val TAG = "AdbGate.Service"
    private val CHANNEL_ID = "adb-gate"
    private val NOTIFICATION_ID = 4711
    private lateinit var scope: CoroutineScope
    private var wsClient: WsClient? = null
    private lateinit var discovery: PairingDiscovery
    private lateinit var store: EncryptedConfigStore

    companion object {
        const val ACTION_START = "se.premex.adbgate.action.START"
        const val ACTION_STOP = "se.premex.adbgate.action.STOP"
    }

    override fun onCreate() {
        super.onCreate()
        scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        store = EncryptedConfigStore(this)
        discovery = PairingDiscovery(this)
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> start()
            ACTION_STOP -> stopSelfAndWs()
            else -> stopSelfAndWs()
        }
        return START_NOT_STICKY
    }

    private fun start() {
        val cfg = store.load()
        if (cfg == null) { Log.w(TAG, "no config; stopping"); stopSelf(); return }
        startInForeground(cfg.serverHost, cfg.nickname)
        AdbWifi.setEnabled(this, true)
        wsClient = WsClient(cfg.serverHost, cfg.wsPort, cfg.nickname, cfg.pskBase64, appVersion(), this).also { it.start() }
    }

    private fun stopSelfAndWs() {
        wsClient?.stop()
        wsClient = null
        AdbWifi.setEnabled(this, false)
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    override fun onDestroy() { scope.cancel(); super.onDestroy() }
    override fun onBind(intent: Intent?): IBinder? = null

    private fun startInForeground(host: String, nickname: String) {
        val n = Notification.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.notification_title))
            .setContentText(getString(R.string.notification_text, host, nickname))
            .setSmallIcon(android.R.drawable.stat_sys_data_bluetooth)
            .setOngoing(true)
            .build()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(NOTIFICATION_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else startForeground(NOTIFICATION_ID, n)
    }

    private fun createChannel() {
        val nm = getSystemService(NotificationManager::class.java)
        val ch = NotificationChannel(CHANNEL_ID, getString(R.string.notification_channel), NotificationManager.IMPORTANCE_LOW)
        nm.createNotificationChannel(ch)
    }

    private fun appVersion(): String = try {
        packageManager.getPackageInfo(packageName, 0).versionName ?: "?"
    } catch (e: Exception) { "?" }

    override fun onPrepConnect(requestPair: Boolean) {
        scope.launch {
            AdbWifi.setEnabled(this@AdbGateService, true)
            kotlinx.coroutines.delay(800)
            val svcType = if (requestPair) "_adb-tls-pairing._tcp" else "_adb-tls-connect._tcp"
            val ep = discovery.discover(svcType) ?: run {
                Log.w(TAG, "mDNS discover failed for $svcType")
                wsClient?.sendError("discover_failed", "mDNS discovery timed out for $svcType — is wireless debugging visible on the phone?")
                return@launch
            }
            val pairCode = if (requestPair) {
                val deferred = PairCodeBridge.beginRequest()
                try { deferred.await() } catch (e: Exception) { null }
            } else null
            // Prefer this phone's Tailscale (100.x) IP over the mDNS-discovered wlan0 IP.
            // adbd binds on all interfaces, so the port is reachable via any of them; reporting
            // the Tailscale IP is what makes the daemon's adb pair/connect route through the tailnet.
            val host = tailscaleIpv4() ?: ep.host
            Log.i(TAG, "connect_ready host=$host port=${ep.port} (mdns host was ${ep.host})")
            wsClient?.sendConnectReady(host, ep.port, pairCode)
        }
    }

    private fun tailscaleIpv4(): String? {
        return try {
            val interfaces = java.net.NetworkInterface.getNetworkInterfaces().toList()
            // Tailscale Android creates a "tun*" interface. Match by name first to avoid
            // confusing it with carrier CGNAT ranges that also live in 100.64.0.0/10.
            val tunIfaces = interfaces.filter { it.name.startsWith("tun") && it.isUp }
            for (iface in tunIfaces) {
                val addr = iface.inetAddresses.toList()
                    .filterIsInstance<java.net.Inet4Address>()
                    .map { it.hostAddress }
                    .firstOrNull { it != null }
                if (addr != null) return addr
            }
            null
        } catch (e: Exception) { null }
    }

    override fun onServerError(code: String, message: String) {
        Log.w(TAG, "server error $code: $message")
        if (code == "auth_failed" || code == "unknown_phone") stopSelfAndWs()
    }
}
