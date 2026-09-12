package com.console.mobile.feature.terminal.native

import android.content.Context
import android.graphics.Color
import android.graphics.Typeface
import android.os.Build
import android.text.Editable
import android.text.InputType
import android.text.TextWatcher
import android.view.KeyEvent
import android.view.ViewGroup
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputConnectionWrapper
import android.view.inputmethod.InputMethodManager
import android.widget.EditText
import android.widget.FrameLayout
import kotlin.math.max

/**
 * Plain-Android port of apps/mobile/modules/console-terminal's Expo
 * ConsoleTerminalView: owns the Ghostty JNI terminal handle, renders via
 * TerminalCanvasView, and bridges IME input through a 1x1 hidden EditText.
 * No Expo module wrapper — Compose hosts this directly via AndroidView.
 */
class NativeTerminalView(context: Context) : FrameLayout(context) {
    private val terminalCanvas = TerminalCanvasView(context)
    private val inputView = object : EditText(context) {
        override fun onCreateInputConnection(outAttrs: EditorInfo?): InputConnection? {
            val connection = super.onCreateInputConnection(outAttrs) ?: return null
            return PtyInputConnection(connection)
        }
    }

    var onInput: ((String) -> Unit)? = null
    var onResize: ((cols: Int, rows: Int) -> Unit)? = null

    private var terminalHandle = 0L
    private var fedBuffer = ""
    private var cols = 0
    private var rows = 0
    private var clearingInput = false
    private var isCleanedUp = false
    private var backgroundColorValue = Color.parseColor("#0a0a0b")
    private var foregroundColorValue = Color.parseColor("#ffffff")
    private var cursorColorValue = Color.parseColor("#009FFF")
    private var paletteColors = IntArray(0)

    var fontSize: Float = 13f
        set(value) {
            field = value
            terminalCanvas.fontSizeSp = value
            inputView.textSize = max(value, 13f)
            emitResize()
        }

    var initialBuffer: String = ""
        set(value) {
            if (field == value) return
            field = value
            feedPendingBuffer()
        }

    init {
        terminalCanvas.fontSizeSp = fontSize
        terminalCanvas.onRequestKeyboard = { requestKeyboardFocus() }
        terminalCanvas.onScrollRows = { delta ->
            if (terminalHandle != 0L) {
                GhosttyBridge.nativeScroll(terminalHandle, delta)
                renderSnapshot()
            }
            if (delta < 0 && softKeyboardShowing()) {
                hideKeyboard()
                inputView.clearFocus()
            }
        }
        terminalCanvas.onCellMetricsChanged = { emitResize() }
        terminalCanvas.selectionDelegate = object : TerminalSelectionDelegate {
            override fun selectWordAt(col: Int, row: Int): Boolean {
                if (terminalHandle == 0L) return false
                val selected = GhosttyBridge.nativeSelectWordAt(terminalHandle, col, row)
                if (selected) renderSnapshot()
                return selected
            }

            override fun extendSelection(anchorCol: Int, anchorRow: Int, col: Int, row: Int) {
                if (terminalHandle == 0L) return
                GhosttyBridge.nativeExtendSelection(terminalHandle, anchorCol, anchorRow, col, row)
                renderSnapshot()
            }

            override fun selectAll(): Boolean {
                if (terminalHandle == 0L) return false
                val selected = GhosttyBridge.nativeSelectAll(terminalHandle)
                if (selected) renderSnapshot()
                return selected
            }

            override fun clearSelection() {
                if (terminalHandle == 0L) return
                GhosttyBridge.nativeClearSelection(terminalHandle)
                renderSnapshot()
            }

            override fun selectionText(): String? =
                if (terminalHandle == 0L) {
                    null
                } else {
                    GhosttyBridge.nativeGetSelectionText(terminalHandle)?.let { String(it, Charsets.UTF_8) }
                }
        }

        configureInputView()
        addView(terminalCanvas, LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        addView(inputView, LayoutParams(1, 1))
        applyTheme()
    }

    override fun onSizeChanged(width: Int, height: Int, oldWidth: Int, oldHeight: Int) {
        super.onSizeChanged(width, height, oldWidth, oldHeight)
        if (width != oldWidth || height != oldHeight) emitResize()
    }

    fun requestKeyboardFocus() {
        inputView.requestFocus()
        val imm = context.getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager
        imm?.showSoftInput(inputView, InputMethodManager.SHOW_IMPLICIT)
    }

    fun hideKeyboard() {
        val imm = context.getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager
        imm?.hideSoftInputFromWindow(windowToken, 0)
    }

    fun cleanup() {
        if (isCleanedUp) return
        isCleanedUp = true
        inputView.setOnEditorActionListener(null)
        terminalCanvas.onScrollRows = null
        terminalCanvas.onRequestKeyboard = null
        terminalCanvas.onCellMetricsChanged = null
        terminalCanvas.selectionDelegate = null
        destroyTerminal()
    }

    private fun configureInputView() {
        inputView.setSingleLine(true)
        inputView.setTextColor(Color.TRANSPARENT)
        inputView.setHintTextColor(Color.TRANSPARENT)
        inputView.setBackgroundColor(Color.TRANSPARENT)
        inputView.typeface = Typeface.MONOSPACE
        inputView.textSize = max(fontSize, 13f)
        inputView.alpha = 0.01f
        inputView.isFocusableInTouchMode = true
        inputView.imeOptions = EditorInfo.IME_ACTION_SEND or
            EditorInfo.IME_FLAG_NO_EXTRACT_UI or
            EditorInfo.IME_FLAG_NO_FULLSCREEN or
            EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING
        inputView.inputType = InputType.TYPE_CLASS_TEXT or
            InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD or
            InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
        inputView.setPadding(0, 0, 0, 0)
        inputView.setOnEditorActionListener { _, actionId, event ->
            val isImeSend = event == null && actionId == EditorInfo.IME_ACTION_SEND
            val isEnterKey = event?.keyCode == KeyEvent.KEYCODE_ENTER
            if (!isImeSend && !isEnterKey) {
                return@setOnEditorActionListener false
            }
            if (event == null || event.action == KeyEvent.ACTION_DOWN) {
                onInput?.invoke("\r")
                keepFocusAfterInput()
            }
            true
        }
        inputView.setOnKeyListener { _, keyCode, event ->
            if (event.action != KeyEvent.ACTION_DOWN) return@setOnKeyListener false
            when {
                keyCode == KeyEvent.KEYCODE_DEL -> {
                    onInput?.invoke("\u007F")
                    keepFocusAfterInput()
                    true
                }
                event.isCtrlPressed && keyCode in KeyEvent.KEYCODE_A..KeyEvent.KEYCODE_Z -> {
                    onInput?.invoke((keyCode - KeyEvent.KEYCODE_A + 1).toChar().toString())
                    keepFocusAfterInput()
                    true
                }
                else -> false
            }
        }
        inputView.addTextChangedListener(
            object : TextWatcher {
                override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit

                override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                    if (clearingInput || s == null || count <= 0) return
                    val end = (start + count).coerceAtMost(s.length)
                    if (start >= end) return
                    val insertedText = s.subSequence(start, end).toString()
                    if (insertedText.isNotEmpty()) {
                        onInput?.invoke(insertedText)
                    }
                }

                override fun afterTextChanged(editable: Editable?) {
                    if (clearingInput || editable.isNullOrEmpty()) return
                    clearingInput = true
                    editable.clear()
                    clearingInput = false
                }
            },
        )
    }

    private fun keepFocusAfterInput() {
        inputView.requestFocus()
    }

    private fun softKeyboardShowing(): Boolean {
        val inputMethodManager = context.getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager ?: return false
        return if (Build.VERSION.SDK_INT >= 30) {
            inputMethodManager.isAcceptingText
        } else {
            inputView.isFocused
        }
    }

    private inner class PtyInputConnection(
        delegate: InputConnection,
    ) : InputConnectionWrapper(delegate, true) {
        override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
            if (afterLength == 0 && beforeLength > 0 && inputView.text.isNullOrEmpty()) {
                repeat(beforeLength) {
                    onInput?.invoke("\u007F")
                    keepFocusAfterInput()
                }
                return true
            }
            return super.deleteSurroundingText(beforeLength, afterLength)
        }
    }

    @Suppress("ComplexCondition")
    private fun emitResize() {
        if (width <= 0 || height <= 0 || terminalCanvas.width <= 0 || terminalCanvas.height <= 0 || isCleanedUp) {
            return
        }
        val nextCols = (terminalCanvas.usableWidth() / terminalCanvas.cellWidthPx).toInt().coerceIn(2, 400)
        val nextRows = (terminalCanvas.usableHeight() / terminalCanvas.cellHeightPx).toInt().coerceIn(2, 200)
        if (nextCols == cols && nextRows == rows && terminalHandle != 0L) return
        cols = nextCols
        rows = nextRows
        val response = if (terminalHandle == 0L) {
            createTerminal()
            ByteArray(0)
        } else {
            GhosttyBridge.nativeResize(terminalHandle, cols, rows, terminalCanvas.cellWidthPx.toInt(), terminalCanvas.cellHeightPx.toInt())
        }
        emitResponse(response)
        onResize?.invoke(cols, rows)
        feedPendingBuffer()
        renderSnapshot()
    }

    @Suppress("ComplexCondition")
    private fun createTerminal() {
        if (terminalHandle != 0L || cols <= 0 || rows <= 0 || isCleanedUp) return
        terminalHandle = GhosttyBridge.nativeCreate(
            cols,
            rows,
            terminalCanvas.cellWidthPx.toInt(),
            terminalCanvas.cellHeightPx.toInt(),
            foregroundColorValue,
            backgroundColorValue,
            cursorColorValue,
            paletteColors,
        )
        fedBuffer = ""
    }

    private fun destroyTerminal() {
        if (terminalHandle == 0L) return
        GhosttyBridge.nativeDestroy(terminalHandle)
        terminalHandle = 0L
        fedBuffer = ""
        terminalCanvas.resetSelectionState()
    }

    private fun feedPendingBuffer() {
        if (terminalHandle == 0L || initialBuffer == fedBuffer) return
        if (!initialBuffer.startsWith(fedBuffer)) {
            destroyTerminal()
            createTerminal()
            if (terminalHandle == 0L) return
        }
        val suffix = initialBuffer.substring(fedBuffer.length)
        if (suffix.isNotEmpty()) {
            emitResponse(GhosttyBridge.nativeFeed(terminalHandle, suffix.toByteArray(Charsets.UTF_8)))
            if (terminalCanvas.hasActiveSelection()) {
                GhosttyBridge.nativeClearSelection(terminalHandle)
                terminalCanvas.resetSelectionState()
            }
        }
        fedBuffer = initialBuffer
        renderSnapshot()
    }

    private fun renderSnapshot() {
        if (terminalHandle == 0L) return
        TerminalFrame.decode(GhosttyBridge.nativeSnapshot(terminalHandle))?.let(terminalCanvas::setFrame)
    }

    private fun emitResponse(response: ByteArray) {
        if (response.isEmpty()) return
        // PTY-write responses (e.g. cursor position reports) flow back through
        // the same onInput channel the caller already wires to the repository.
        onInput?.invoke(String(response, Charsets.UTF_8))
    }

    private fun applyTheme() {
        setBackgroundColor(backgroundColorValue)
        terminalCanvas.setBackgroundColor(backgroundColorValue)
        if (terminalHandle != 0L) {
            GhosttyBridge.nativeSetTheme(terminalHandle, foregroundColorValue, backgroundColorValue, cursorColorValue, paletteColors)
            renderSnapshot()
        }
    }
}
