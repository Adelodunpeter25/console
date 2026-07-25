import SwiftUI
import AppKit

/// Key commands routed from the editor to the composer (slash-command palette,
/// future pickers). The composer returns `true` to consume the key.
enum ComposerKeyCommand {
    case moveSelectionUp
    case moveSelectionDown
    case acceptSelection
    case dismissSelection
}

/// A multiline text editor where **Return submits** and **Shift+Return inserts
/// a newline**. Grows with its content between `minHeight` and `maxHeight`.
/// Adapted from Codevisor's ChatInputEditor for macOS 13 / Swift 5.9 — no
/// WritingTools or Liquid Glass dependencies.
struct ChatInputEditor: NSViewRepresentable {
    private static let verticalTextInset: CGFloat = 6

    /// Starting at the same height TextKit will report prevents a newly
    /// mounted composer from growing one frame later.
    static var singleLineHeight: CGFloat {
        NSLayoutManager().defaultLineHeight(
            for: NSFont.preferredFont(forTextStyle: .body)
        ) + verticalTextInset * 2
    }

    @Binding var text: String
    @Binding var calculatedHeight: CGFloat
    var minHeight: CGFloat = Self.singleLineHeight
    var maxHeight: CGFloat = 160
    var onSubmit: () -> Void
    var onKeyCommand: ((ComposerKeyCommand) -> Bool)? = nil
    /// Honors SwiftUI `.disabled(...)`: the text view stops accepting edits
    /// (and Return stops submitting) while a send is being accepted.
    @Environment(\.isEnabled) private var isEnabled

    func makeNSView(context: Context) -> NSScrollView {
        let textView = SubmittingTextView()
        textView.delegate = context.coordinator
        textView.onSubmit = { onSubmit() }
        textView.onKeyCommand = onKeyCommand
        textView.string = text
        textView.font = .preferredFont(forTextStyle: .body)
        textView.isRichText = false
        // No automatic formatting in a prompt box: smart quotes/dashes and
        // autocorrect mangle code and identifiers.
        textView.isAutomaticQuoteSubstitutionEnabled = false
        textView.isAutomaticDashSubstitutionEnabled = false
        textView.isAutomaticTextReplacementEnabled = false
        textView.isAutomaticSpellingCorrectionEnabled = false
        textView.isAutomaticLinkDetectionEnabled = false
        textView.isAutomaticDataDetectionEnabled = false
        textView.allowsUndo = true
        textView.drawsBackground = false
        textView.textContainerInset = NSSize(width: 0, height: Self.verticalTextInset)
        textView.isVerticallyResizable = true
        textView.isHorizontallyResizable = false
        textView.textContainer?.widthTracksTextView = true
        textView.textContainer?.lineFragmentPadding = 0

        let scroll = NSScrollView()
        scroll.drawsBackground = false
        scroll.hasVerticalScroller = false
        scroll.hasHorizontalScroller = false
        scroll.documentView = textView
        context.coordinator.textView = textView
        // Height depends on the wrap width, which SwiftUI only settles after
        // layout — re-measure whenever the text view's width changes.
        textView.postsFrameChangedNotifications = true
        NotificationCenter.default.addObserver(
            context.coordinator,
            selector: #selector(Coordinator.textViewFrameDidChange(_:)),
            name: NSView.frameDidChangeNotification,
            object: textView
        )
        DispatchQueue.main.async { recalculateHeight(textView) }
        return scroll
    }

    func updateNSView(_ scrollView: NSScrollView, context: Context) {
        guard let textView = scrollView.documentView as? SubmittingTextView else { return }
        context.coordinator.parent = self
        context.coordinator.isApplyingUpdate = true
        defer { context.coordinator.isApplyingUpdate = false }
        textView.onSubmit = { onSubmit() }
        textView.onKeyCommand = onKeyCommand
        textView.isEditable = isEnabled
        if textView.string != text {
            textView.string = text
        }
        context.coordinator.recalculateHeightIfNeeded()
    }

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    private func recalculateHeight(_ textView: NSTextView) {
        guard let layoutManager = textView.layoutManager,
              let container = textView.textContainer else { return }
        layoutManager.ensureLayout(for: container)
        let used = layoutManager.usedRect(for: container).height
            + textView.textContainerInset.height * 2
        let clamped = min(max(used, minHeight), maxHeight)
        if abs(clamped - calculatedHeight) > 0.5 {
            DispatchQueue.main.async { calculatedHeight = clamped }
        }
    }

    final class Coordinator: NSObject, NSTextViewDelegate {
        var parent: ChatInputEditor
        weak var textView: SubmittingTextView?
        var isApplyingUpdate = false
        private var measuredText: String?
        private var measuredWidth: CGFloat = -1

        init(_ parent: ChatInputEditor) { self.parent = parent }

        deinit { NotificationCenter.default.removeObserver(self) }

        func recalculateHeightIfNeeded() {
            guard let textView else { return }
            let width = textView.bounds.width
            guard measuredText != textView.string || abs(measuredWidth - width) > 0.5 else { return }
            recordMeasurement(textView)
            parent.recalculateHeight(textView)
        }

        @objc func textViewFrameDidChange(_ notification: Notification) {
            guard let textView else { return }
            guard abs(measuredWidth - textView.bounds.width) > 0.5 else { return }
            recordMeasurement(textView)
            parent.recalculateHeight(textView)
        }

        private func recordMeasurement(_ textView: SubmittingTextView) {
            measuredText = textView.string
            measuredWidth = textView.bounds.width
        }

        func textDidChange(_ notification: Notification) {
            guard let textView = notification.object as? NSTextView else { return }
            parent.text = textView.string
            recordMeasurement(textView as! SubmittingTextView)
            parent.recalculateHeight(textView)
        }

        func textView(_ textView: NSTextView, doCommandBy commandSelector: Selector) -> Bool {
            switch commandSelector {
            case #selector(NSResponder.moveUp(_:)):
                return parent.onKeyCommand?(.moveSelectionUp) == true
            case #selector(NSResponder.moveDown(_:)):
                return parent.onKeyCommand?(.moveSelectionDown) == true
            case #selector(NSResponder.insertTab(_:)):
                return parent.onKeyCommand?(.acceptSelection) == true
            case #selector(NSResponder.cancelOperation(_:)):
                return parent.onKeyCommand?(.dismissSelection) == true
            default:
                return false
            }
        }
    }
}

/// An `NSTextView` that submits on Return and inserts a newline on
/// Shift+Return. Menu navigation (arrows, Tab, Escape) is handled by the
/// coordinator's `textView(_:doCommandBy:)`; only Return needs special-casing
/// here because the Shift modifier isn't visible at the command-selector level.
final class SubmittingTextView: NSTextView {
    var onSubmit: (() -> Void)?
    var onKeyCommand: ((ComposerKeyCommand) -> Bool)?

    override func keyDown(with event: NSEvent) {
        // 53 = Escape. Consume it here as well as in the delegate so it can
        // never fall through to NSTextView's default `complete:` behavior.
        if event.keyCode == 53, onKeyCommand?(.dismissSelection) == true { return }
        // 36 = Return, 76 = numeric keypad Enter.
        if event.keyCode == 36 || event.keyCode == 76 {
            guard isEditable else { return }
            if event.modifierFlags.contains(.shift) {
                super.keyDown(with: event) // newline
            } else {
                if onKeyCommand?(.acceptSelection) != true {
                    onSubmit?()
                }
            }
            return
        }
        super.keyDown(with: event)
    }
}
