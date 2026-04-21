package se.premex.adbgate.ws

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.concurrent.TimeUnit
import kotlin.coroutines.CoroutineContext

sealed class WsState {
    data object Idle : WsState()
    data object Connecting : WsState()
    data object Connected : WsState()
    data class Error(val message: String) : WsState()
}

interface WsEvents {
    fun onPrepConnect(requestPair: Boolean)
    fun onServerError(code: String, message: String)
}

class WsClient(
    private val serverHost: String,
    private val wsPort: Int,
    private val nickname: String,
    private val pskBase64: String,
    private val appVersion: String,
    private val events: WsEvents,
    private val coroutineContext: CoroutineContext = Dispatchers.IO,
) {
    private val TAG = "AdbGate.WsClient"
    private val client = OkHttpClient.Builder().pingInterval(20, TimeUnit.SECONDS).build()
    private var ws: WebSocket? = null
    private val backoff = WsBackoff()
    private val _state = MutableStateFlow<WsState>(WsState.Idle)
    val state: StateFlow<WsState> = _state
    private var scope: CoroutineScope? = null
    private var shouldRun = false

    fun start() {
        if (shouldRun) return
        shouldRun = true
        scope = CoroutineScope(coroutineContext)
        scope!!.launch { connectLoop() }
    }

    fun stop() {
        shouldRun = false
        ws?.close(1000, "client stop")
        ws = null
        _state.value = WsState.Idle
    }

    fun sendToggleState(on: Boolean) { ws?.send(WsProtocol.encodeToggle(on)) }
    fun sendConnectReady(ip: String, port: Int, pairCode: String?) {
        ws?.send(WsProtocol.encodeConnectReady(ip, port, pairCode))
    }
    fun sendError(code: String, message: String) {
        ws?.send(WsProtocol.encodeError(code, message))
    }

    private suspend fun connectLoop() {
        while (shouldRun) {
            _state.value = WsState.Connecting
            val connected = connectOnce()
            if (!connected && shouldRun) {
                val d = backoff.nextDelayMs()
                Log.w(TAG, "reconnect in ${d}ms")
                delay(d)
            }
        }
    }

    private suspend fun connectOnce(): Boolean {
        val deferredResult = kotlinx.coroutines.CompletableDeferred<Boolean>()
        val req = Request.Builder().url("ws://$serverHost:$wsPort").build()
        val listener = object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                ws = webSocket
                webSocket.send(WsProtocol.encodeHello(nickname, pskBase64, appVersion))
            }
            override fun onMessage(webSocket: WebSocket, text: String) {
                when (val f = WsProtocol.decodeServerFrame(text)) {
                    is ServerFrame.Ack -> { _state.value = WsState.Connected; backoff.reset() }
                    is ServerFrame.PrepConnect -> events.onPrepConnect(f.requestPair)
                    is ServerFrame.Error -> events.onServerError(f.code, f.message)
                    is ServerFrame.Unknown -> Log.w(TAG, "unknown frame: $text")
                }
            }
            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) { webSocket.close(1000, null) }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                _state.value = WsState.Idle
                if (!deferredResult.isCompleted) deferredResult.complete(false)
            }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                _state.value = WsState.Error(t.message ?: "unknown")
                if (!deferredResult.isCompleted) deferredResult.complete(false)
            }
        }
        client.newWebSocket(req, listener)
        return deferredResult.await()
    }
}
