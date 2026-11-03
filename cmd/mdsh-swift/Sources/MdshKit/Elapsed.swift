import Foundation

/// elapsed says how long a command has run for, as mdsh-gui's History
/// clock does: tenths of a second for the first minute, then minutes
/// and seconds, then hours. Whole seconds when tenths is false, for
/// reduced motion, as the clock ticks less.
public func elapsed(_ seconds: TimeInterval, tenths: Bool = true) -> String {
    let ms = Int((seconds * 1000).rounded(.down))
    let s = ms / 1000
    if s < 60 { return tenths ? String(format: "%.1fs", Double(ms / 100) / 10) : "\(s)s" }
    func pad(_ n: Int) -> String { n < 10 ? "0\(n)" : "\(n)" }
    if s < 3600 { return "\(s / 60)m \(pad(s % 60))s" }
    return "\(s / 3600)h \(pad(s / 60 % 60))m \(pad(s % 60))s"
}

/// isNote says History input is a note, every line starting with #,
/// which is added as prose rather than run.
public func isHistoryNote(_ text: String) -> Bool {
    text.split(separator: "\n", omittingEmptySubsequences: false).allSatisfy { $0.drop(while: \.isWhitespace).hasPrefix("#") }
}

/// limitTime says a timeout as it is written, more or less: 30s, 1.5s,
/// 5m, 1h, or as elapsed does when it doesn't come out even.
public func limitTime(_ ms: Int) -> String {
    let s = Double(ms) / 1000
    if s < 60 {
        let r = (s * 10).rounded() / 10
        return (r == r.rounded() ? String(Int(r)) : String(r)) + "s"
    }
    if ms % 3_600_000 == 0 { return "\(ms / 3_600_000)h" }
    if ms % 60_000 == 0 { return "\(ms / 60_000)m" }
    return elapsed(s)
}
