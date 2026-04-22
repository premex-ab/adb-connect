package se.premex.adbgate

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import se.premex.adbgate.ui.MainScreen
import se.premex.adbgate.ui.theme.AdbGateTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { AdbGateTheme { MainScreen() } }
    }
}
