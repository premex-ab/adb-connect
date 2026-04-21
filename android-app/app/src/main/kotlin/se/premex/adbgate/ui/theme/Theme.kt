package se.premex.adbgate.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable

@Composable
fun AdbGateTheme(darkTheme: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val colors = darkColorScheme(
        primary = PremexOrange,
        background = PremexBg,
        surface = PremexCharcoal,
        onPrimary = PremexFg,
        onBackground = PremexFg,
        onSurface = PremexFg,
    )
    MaterialTheme(colorScheme = colors, typography = AppTypography, content = content)
}
