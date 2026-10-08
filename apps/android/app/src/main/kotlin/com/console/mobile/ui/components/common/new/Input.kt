package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Text
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme

/** A form field: accent heading over a [Section] card holding borderless text input. */
@Composable
fun TextInput(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    singleLine: Boolean = true,
    monospace: Boolean = false,
    keyboardType: KeyboardType = KeyboardType.Text,
) {
    Section(label) {
        TextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = { Text(placeholder, color = NewTheme.TextGhost, fontSize = 15.sp) },
            singleLine = singleLine,
            keyboardOptions = KeyboardOptions(keyboardType = keyboardType),
            textStyle = TextStyle(
                color = NewTheme.TextPrimary, fontSize = 16.sp,
                fontFamily = if (monospace) ConsoleMonoFamily else null,
            ),
            colors = TextFieldDefaults.colors(
                focusedContainerColor = Color.Transparent,
                unfocusedContainerColor = Color.Transparent,
                disabledContainerColor = Color.Transparent,
                focusedIndicatorColor = Color.Transparent,
                unfocusedIndicatorColor = Color.Transparent,
                disabledIndicatorColor = Color.Transparent,
                cursorColor = NewTheme.Accent,
            ),
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

/**
 * A choice between a few options, as pills in a [Section] card. The selected
 * option takes the accent.
 */
@Composable
fun <T> ChoiceGroup(
    label: String,
    options: List<Pair<T, String>>,
    selected: T,
    onSelect: (T) -> Unit,
) {
    Section(label) {
        Row(modifier = Modifier.fillMaxWidth().padding(10.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            options.forEach { (value, text) ->
                val on = value == selected
                Box(
                    modifier = Modifier.weight(1f).clip(RoundedCornerShape(NewTheme.ChipRadius))
                        .background(if (on) NewTheme.Accent.copy(alpha = 0.18f) else Color.White.copy(alpha = 0.05f))
                        .clickable { onSelect(value) }.padding(vertical = 12.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(text, color = if (on) NewTheme.Accent else NewTheme.TextSecondary, fontSize = 14.sp, fontWeight = if (on) FontWeight.SemiBold else FontWeight.Normal, maxLines = 1)
                }
            }
        }
    }
}

/**
 * A text field with no heading, for panels that already say what it is for (an
 * answer box under a question). Filled with [NewTheme.Raised] so it reads on a
 * [NewTheme.Card] surface.
 */
@Composable
fun InlineTextInput(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    modifier: Modifier = Modifier,
    maxLines: Int = 4,
) {
    TextField(
        value = value,
        onValueChange = onValueChange,
        placeholder = { Text(placeholder, color = NewTheme.TextGhost, fontSize = 15.sp) },
        maxLines = maxLines,
        textStyle = TextStyle(color = NewTheme.TextPrimary, fontSize = 16.sp),
        shape = RoundedCornerShape(NewTheme.InputRadius),
        colors = TextFieldDefaults.colors(
            focusedContainerColor = NewTheme.Raised,
            unfocusedContainerColor = NewTheme.Raised,
            disabledContainerColor = NewTheme.Raised,
            focusedIndicatorColor = Color.Transparent,
            unfocusedIndicatorColor = Color.Transparent,
            disabledIndicatorColor = Color.Transparent,
            cursorColor = NewTheme.Accent,
        ),
        modifier = modifier.fillMaxWidth(),
    )
}
