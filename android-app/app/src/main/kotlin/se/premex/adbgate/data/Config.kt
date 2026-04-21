package se.premex.adbgate.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

@Serializable
data class Config(
    val nickname: String,
    val serverHost: String,
    val wsPort: Int,
    val pskBase64: String,
) {
    companion object {
        fun fromQrPayload(json: String, nickname: String): Config {
            val obj = Json.parseToJsonElement(json) as JsonObject
            fun s(k: String) = (obj[k] as JsonPrimitive).content
            fun i(k: String) = (obj[k] as JsonPrimitive).content.toInt()
            require(i("v") == 1) { "unsupported enrollment payload version" }
            return Config(nickname = nickname, serverHost = s("host"), wsPort = i("port"), pskBase64 = s("psk"))
        }
    }
}
