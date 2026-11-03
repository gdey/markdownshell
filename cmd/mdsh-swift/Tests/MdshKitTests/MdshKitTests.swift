import Foundation
import XCTest

@testable import MdshKit

final class ModelTests: XCTestCase {
    func testCellDecodes() throws {
        // A prose cell and a block cell as notebook.Cell encodes them,
        // with omitempty fields left out.
        let json = """
            [{"block":-1,"html":"<h1>A</h1>","ran":false,"status":0,"failed":false,"running":false,"heading":1,
              "sectionFrom":0,"sectionTo":1,"from":0,"to":4,"line":1,"dropAfter":-1,"dropBefore":-1},
             {"block":0,"id":"hi","name":"hi","lang":"sh","body":"echo hi\\n","output":"<pre>hi</pre>","ran":true,"stale":true,"nosave":true,
              "status":0,"failed":false,"running":false,"from":5,"to":30,"line":3,"movable":true,"dropAfter":-1,"dropBefore":-1}]
            """
        let cells = try JSONDecoder().decode([Cell].self, from: Data(json.utf8))
        XCTAssertEqual(cells.count, 2)
        XCTAssertTrue(cells[0].isHeading)
        XCTAssertTrue(cells[0].hasSection)
        XCTAssertFalse(cells[0].isBlock)
        XCTAssertEqual(cells[1].blockID, "hi")
        XCTAssertEqual(cells[1].id, 5)
        XCTAssertEqual(cells[1].movable, true)
        XCTAssertNil(cells[0].stale)
        XCTAssertEqual(cells[1].stale, true)
        XCTAssertEqual(cells[1].nosave, true)
    }

    func testElapsed() {
        XCTAssertEqual(elapsed(0), "0.0s")
        XCTAssertEqual(elapsed(12.46), "12.4s")
        XCTAssertEqual(elapsed(12.46, tenths: false), "12s")
        XCTAssertEqual(elapsed(65), "1m 05s")
        XCTAssertEqual(elapsed(3725), "1h 02m 05s")
        XCTAssertEqual(limitTime(30000), "30s")
        XCTAssertEqual(limitTime(1500), "1.5s")
        XCTAssertEqual(limitTime(300_000), "5m")
        XCTAssertEqual(limitTime(3_600_000), "1h")
        XCTAssertEqual(limitTime(90_000), "1m 30s")
        XCTAssertTrue(isHistoryNote("# a\n  # b"))
        XCTAssertFalse(isHistoryNote("# a\nls"))
    }

    func testReadline() {
        var r = Readline()
        let s = "ls -la /tmp/x y"
        let end = NSRange(location: (s as NSString).length, length: 0)
        // ctrl+w takes a path or flag whole.
        XCTAssertEqual(r.key(.ctrl("w"), text: "cd /tmp/a b", selection: NSRange(location: 9, length: 0)), .replace(NSRange(location: 3, length: 6), ""))
        r.other()
        // Kills one after another add to the kill text.
        XCTAssertEqual(r.key(.ctrl("w"), text: s, selection: end), .replace(NSRange(location: 14, length: 1), ""))
        XCTAssertEqual(r.key(.ctrl("w"), text: "ls -la /tmp/x ", selection: NSRange(location: 14, length: 0)), .replace(NSRange(location: 7, length: 7), ""))
        XCTAssertEqual(r.killed, "/tmp/x y")
        r.other()
        XCTAssertEqual(r.key(.ctrl("u"), text: "a\nbc d", selection: NSRange(location: 5, length: 0)), .replace(NSRange(location: 2, length: 3), ""))
        XCTAssertEqual(r.killed, "bc ")
        XCTAssertEqual(r.key(.ctrl("y"), text: "x", selection: NSRange(location: 1, length: 0)), .replace(NSRange(location: 1, length: 0), "bc "))
        XCTAssertEqual(r.key(.ctrl("k"), text: "ab\ncd", selection: NSRange(location: 2, length: 0)), .replace(NSRange(location: 2, length: 1), ""))
        XCTAssertEqual(r.key(.alt("b"), text: s, selection: end), .move(14))
        XCTAssertEqual(r.key(.alt("f"), text: s, selection: NSRange(location: 0, length: 0)), .move(2))
        XCTAssertEqual(r.key(.alt("d"), text: s, selection: NSRange(location: 2, length: 0)), .replace(NSRange(location: 2, length: 4), ""))
        XCTAssertEqual(r.key(.alt("backspace"), text: s, selection: end), .replace(NSRange(location: 14, length: 1), ""))
        XCTAssertEqual(r.key(.ctrl("w"), text: "", selection: NSRange(location: 0, length: 0)), .nothing)
        XCTAssertEqual(r.key(.ctrl("d"), text: "", selection: NSRange(location: 0, length: 0)), .eof)
        XCTAssertNil(r.key(.ctrl("d"), text: "a", selection: NSRange(location: 0, length: 0)))
        XCTAssertEqual(r.key(.ctrl("p"), text: "one", selection: NSRange(location: 1, length: 0)), .prev)
        XCTAssertEqual(r.key(.ctrl("p"), text: "one\ntwo", selection: NSRange(location: 6, length: 0)), .move(2))
        XCTAssertEqual(r.key(.ctrl("n"), text: "one\ntwo", selection: NSRange(location: 6, length: 0)), .next)
        XCTAssertEqual(r.key(.ctrl("l"), text: "", selection: NSRange(location: 0, length: 0)), .clear)
        XCTAssertNil(r.key(.ctrl("a"), text: s, selection: end))
    }

    func testBlockEventBytes() throws {
        // Go encodes []byte as base64.
        let e = try JSONDecoder().decode(BlockEvent.self, from: Data(#"{"kind":"tty","block":2,"bytes":"aGk="}"#.utf8))
        XCTAssertEqual(e.bytes.flatMap { String(data: $0, encoding: .utf8) }, "hi")
    }

    func testSettingsRoundTrip() throws {
        var s = AppSettings()
        s.look = "tty"
        s.textSize = 120
        s.historyRC = "~/.zshrc"
        let data = try JSONEncoder().encode(s)
        let obj = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        // The keys are Go's json tags.
        XCTAssertEqual(obj?["historyRC"] as? String, "~/.zshrc")
        XCTAssertEqual(obj?["textSize"] as? Int, 120)
        let back = try JSONDecoder().decode(AppSettings.self, from: data)
        XCTAssertEqual(back, s)
        // A settings file from an older mdsh-gui, missing keys, still reads.
        let old = try JSONDecoder().decode(AppSettings.self, from: Data(#"{"autosave":true}"#.utf8))
        XCTAssertTrue(old.autosave)
        XCTAssertEqual(old.look, "")
    }

    func testJSONValueIntegers() throws {
        // Go's int fields refuse 1.0, so whole numbers go out as integers.
        let data = try JSONEncoder().encode(["doc": JSONValue(3), "on": JSONValue(true)] as [String: JSONValue])
        let s = String(decoding: data, as: UTF8.self)
        XCTAssertTrue(s.contains(#""doc":3"#), s)
    }
}

/// BridgeTests drive a real bridge, when MDSH_SWIFT_BRIDGE names one,
/// as CI does after building it.
final class BridgeTests: XCTestCase {
    private func bridge() throws -> Bridge {
        guard let path = ProcessInfo.processInfo.environment["MDSH_SWIFT_BRIDGE"], !path.isEmpty else {
            throw XCTSkip("MDSH_SWIFT_BRIDGE is not set")
        }
        let settings = FileManager.default.temporaryDirectory.appendingPathComponent("mdsh-swift-test-\(UUID()).json")
        return try Bridge(executable: URL(fileURLWithPath: path), arguments: ["--settings", settings.path])
    }

    func testRunBlock() async throws {
        let b = try bridge()
        defer { b.stop() }
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("a.md")
        try "# A\n\n```sh {#hi}\necho hello\n```\n".write(to: file, atomically: true, encoding: .utf8)

        let events = EventLog()
        b.onEvent = { e in events.add(e) }

        let info: DocInfo = try await b.call("open", ["file": .init(file.path)])
        XCTAssertEqual(info.file, file.path)
        let cells: [Cell] = try await b.call("cells", ["doc": .init(info.id)])
        let block = try XCTUnwrap(cells.first { $0.blockID == "hi" }).block
        try await b.call("runBlock", ["doc": .init(info.id), "block": .init(block)])
        let after: [Cell] = try await b.call("cells", ["doc": .init(info.id)])
        let hi = try XCTUnwrap(after.first { $0.blockID == "hi" })
        XCTAssertTrue(hi.ran)
        XCTAssertTrue((hi.output ?? "").contains("hello"), hi.output ?? "")

        // Events come on the main queue; give them a moment.
        try await Task.sleep(nanoseconds: 300_000_000)
        XCTAssertTrue(events.names.contains("ran"), "\(events.names)")

        do {
            try await b.call("nope")
            XCTFail("unknown method answered")
        } catch let e as BridgeError {
            XCTAssertTrue(e.message.contains("unknown method"), e.message)
        }
    }
}

private final class EventLog: @unchecked Sendable {
    private let lock = NSLock()
    private var list: [String] = []
    func add(_ e: BridgeEvent) {
        lock.lock()
        list.append(e.name)
        lock.unlock()
    }
    var names: [String] {
        lock.lock()
        defer { lock.unlock() }
        return list
    }
}

final class TermTextTests: XCTestCase {
    func testBackspaceAndCarriageReturn() {
        var t = TermText()
        // A typo fixed with Backspace, as the fruit tape types it, and a
        // progress line redrawn with \r.
        t.write(Data("fruit> appel\u{8}\u{8}\u{1B}[Kle\r\n".utf8))
        t.write(Data("10%\r50%\r100%\n".utf8))
        XCTAssertEqual(t.text, "fruit> apple\n100%\n")
        t.write(Data("fruit> ".utf8))
        XCTAssertEqual(t.textWithCursor, "fruit> apple\n100%\nfruit> \u{2588}")
    }

    func testSplitWrites() {
        var t = TermText()
        let b = [UInt8]("\u{1B}[31mé\u{1B}[0m".utf8)
        // Cut inside the escape and inside é.
        t.write(Data(b[0..<2]))
        t.write(Data(b[2..<6]))
        t.write(Data(b[6...]))
        XCTAssertEqual(t.text, "é")
    }

    func testStringEscapesAndBadBytes() {
        var t = TermText()
        // A DCS string is dropped whole, a charset pick is 3 bytes, and
        // a bad lead byte is one replacement character.
        t.write(Data("a\u{1B}Ptmux;x\u{1B}\\b\u{1B}(Bc".utf8) + Data([0xF0, 0x41]))
        XCTAssertEqual(t.text, "abc\u{FFFD}A")
    }

    func testOpenEscapeIsDropped() {
        var t = TermText()
        t.write(Data("a\u{1B}]0;".utf8) + Data(repeating: 0x41, count: 5000))
        t.write(Data("b".utf8))
        XCTAssertEqual(t.text, "ab")
    }

    func testStream() {
        var s = TermStream()
        let b = [UInt8]("x\u{1B}[1mé\r\n".utf8)
        XCTAssertEqual(s.take(Data(b[0..<3])), "x")
        XCTAssertEqual(s.take(Data(b[3..<6])), "")
        XCTAssertEqual(s.take(Data(b[6...])), "é\n")
        XCTAssertEqual(s.take(Data("a\r".utf8)), "a")
        XCTAssertEqual(s.take(Data("\nb".utf8)), "\nb")
    }
}
