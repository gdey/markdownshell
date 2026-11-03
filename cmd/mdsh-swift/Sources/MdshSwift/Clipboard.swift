import AppKit

/// Clipboard copies text, as mdsh-gui's copy buttons do. A secret is
/// marked concealed, so clipboard managers that honor
/// org.nspasteboard.ConcealedType leave it out of their history, and is
/// taken off the clipboard after Settings › Editing › Show secrets for,
/// unless something else was copied since.
@MainActor
enum Clipboard {
    private static let concealed = NSPasteboard.PasteboardType("org.nspasteboard.ConcealedType")

    static func copy(_ text: String, secret: Bool = false) {
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(text, forType: .string)
        guard secret else { return }
        pb.setString("", forType: concealed)
        let mine = pb.changeCount
        let after = AppModel.shared.settings.secretShow
        Task { @MainActor in
            try? await Task.sleep(for: .seconds(after))
            if NSPasteboard.general.changeCount == mine { NSPasteboard.general.clearContents() }
        }
    }
}
