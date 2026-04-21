package se.premex.adbgate.data

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.serialization.json.Json

interface ConfigStore {
    fun save(config: Config)
    fun load(): Config?
    fun clear()
    fun saveNickname(nickname: String)
    fun loadNickname(): String?
}

class InMemoryConfigStore : ConfigStore {
    private var config: Config? = null
    private var nickname: String? = null
    override fun save(config: Config) { this.config = config; this.nickname = config.nickname }
    override fun load(): Config? = config
    override fun clear() { config = null }
    override fun saveNickname(nickname: String) { this.nickname = nickname }
    override fun loadNickname(): String? = nickname
}

class EncryptedConfigStore(context: Context) : ConfigStore {
    private val masterKey = MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build()
    private val prefs = EncryptedSharedPreferences.create(
        context, "adb-gate-config", masterKey,
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )
    private val json = Json { ignoreUnknownKeys = true }

    override fun save(config: Config) {
        prefs.edit()
            .putString("config", json.encodeToString(Config.serializer(), config))
            .putString("nickname", config.nickname).apply()
    }
    override fun load(): Config? {
        val s = prefs.getString("config", null) ?: return null
        return try { json.decodeFromString(Config.serializer(), s) } catch (e: Exception) { null }
    }
    override fun clear() { prefs.edit().remove("config").apply() }
    override fun saveNickname(nickname: String) { prefs.edit().putString("nickname", nickname).apply() }
    override fun loadNickname(): String? = prefs.getString("nickname", null)
}
