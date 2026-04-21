package se.premex.adbgate.ws

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.contentOrNull

@Serializable data class HelloFrame(val op: String = "hello", val nickname: String, val psk: String, val app_version: String)
@Serializable data class ToggleStateFrame(val op: String = "toggle_state", val on: Boolean)
@Serializable data class ConnectReadyFrame(val op: String = "connect_ready", val ip: String, val port: Int, val pair_code: String? = null)
@Serializable data class ErrorFrame(val op: String = "error", val code: String, val message: String)

sealed class ServerFrame {
    data class PrepConnect(val requestPair: Boolean) : ServerFrame()
    data object Ack : ServerFrame()
    data class Error(val code: String, val message: String) : ServerFrame()
    data class Unknown(val raw: String) : ServerFrame()
}

object WsProtocol {
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    fun encodeHello(nickname: String, psk: String, appVersion: String): String =
        json.encodeToString(HelloFrame.serializer(), HelloFrame(nickname = nickname, psk = psk, app_version = appVersion))

    fun encodeToggle(on: Boolean): String =
        json.encodeToString(ToggleStateFrame.serializer(), ToggleStateFrame(on = on))

    fun encodeConnectReady(ip: String, port: Int, pairCode: String?): String =
        json.encodeToString(ConnectReadyFrame.serializer(), ConnectReadyFrame(ip = ip, port = port, pair_code = pairCode))

    fun encodeError(code: String, message: String): String =
        json.encodeToString(ErrorFrame.serializer(), ErrorFrame(code = code, message = message))

    fun decodeServerFrame(raw: String): ServerFrame = try {
        val obj = json.parseToJsonElement(raw) as? JsonObject ?: return ServerFrame.Unknown(raw)
        val op = (obj["op"] as? JsonPrimitive)?.contentOrNull ?: return ServerFrame.Unknown(raw)
        when (op) {
            "ack" -> ServerFrame.Ack
            "prep_connect" -> ServerFrame.PrepConnect(
                requestPair = (obj["request_pair"] as? JsonPrimitive)?.boolean ?: false,
            )
            "error" -> ServerFrame.Error(
                code = (obj["code"] as? JsonPrimitive)?.contentOrNull ?: "unknown",
                message = (obj["message"] as? JsonPrimitive)?.contentOrNull ?: "",
            )
            else -> ServerFrame.Unknown(raw)
        }
    } catch (e: Exception) { ServerFrame.Unknown(raw) }
}
