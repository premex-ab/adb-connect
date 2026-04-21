package se.premex.adbgate.adb

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.withTimeoutOrNull

class PairingDiscovery(context: Context) {
    private val nsd = context.getSystemService(Context.NSD_SERVICE) as NsdManager
    data class Endpoint(val host: String, val port: Int)

    suspend fun discover(serviceType: String, timeoutMs: Long = 8_000): Endpoint? {
        val result = CompletableDeferred<Endpoint?>()
        val listener = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(serviceType: String) {}
            override fun onDiscoveryStopped(serviceType: String) {}
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) { result.complete(null) }
            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {}
            override fun onServiceLost(serviceInfo: NsdServiceInfo) {}
            override fun onServiceFound(serviceInfo: NsdServiceInfo) {
                nsd.resolveService(serviceInfo, object : NsdManager.ResolveListener {
                    override fun onResolveFailed(info: NsdServiceInfo, code: Int) {}
                    override fun onServiceResolved(info: NsdServiceInfo) {
                        val host = info.host?.hostAddress ?: return
                        result.complete(Endpoint(host, info.port))
                    }
                })
            }
        }
        nsd.discoverServices(serviceType, NsdManager.PROTOCOL_DNS_SD, listener)
        val ep = withTimeoutOrNull(timeoutMs) { result.await() }
        try { nsd.stopServiceDiscovery(listener) } catch (e: Exception) {}
        return ep
    }

    suspend fun discoverPairing() = discover("_adb-tls-pairing._tcp")
    suspend fun discoverConnect() = discover("_adb-tls-connect._tcp")
}
