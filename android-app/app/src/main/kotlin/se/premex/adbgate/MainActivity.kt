package se.premex.adbgate

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import se.premex.adbgate.data.EncryptedConfigStore
import se.premex.adbgate.ui.AppNavGraph
import se.premex.adbgate.ui.theme.AdbGateTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val store = EncryptedConfigStore(this)
        val start = if (store.load() == null) "onboarding" else "main"
        setContent { AdbGateTheme { AppNavGraph(startDestination = start) } }
    }
}
