package se.premex.adbgate.ui

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import se.premex.adbgate.R
import se.premex.adbgate.data.EncryptedConfigStore

@Composable
fun OnboardingScreen(onReady: () -> Unit) {
    val context = LocalContext.current
    val store = remember { EncryptedConfigStore(context) }
    var nickname by remember { mutableStateOf(store.loadNickname() ?: "") }
    Column(
        Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Premex ADB-gate", style = MaterialTheme.typography.headlineMedium, color = MaterialTheme.colorScheme.primary)
        Text(stringResource(R.string.by_premex), style = MaterialTheme.typography.bodySmall)
        Spacer(Modifier.height(32.dp))
        OutlinedTextField(
            value = nickname,
            onValueChange = { nickname = it.lowercase().replace(Regex("[^a-z0-9-]"), "") },
            label = { Text(stringResource(R.string.nickname_hint)) },
            singleLine = true,
        )
        Spacer(Modifier.height(16.dp))
        Button(
            enabled = nickname.matches(Regex("^[a-z0-9-]{2,32}$")),
            onClick = { store.saveNickname(nickname); onReady() },
        ) { Text(stringResource(R.string.continue_button)) }
    }
}
