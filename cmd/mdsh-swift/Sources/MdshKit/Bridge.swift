import Foundation

/// BridgeError is a call the bridge refused or couldn't answer.
public struct BridgeError: Error, LocalizedError, Sendable {
    public let message: String
    public init(_ message: String) { self.message = message }
    public var errorDescription: String? { message }
}

/// BridgeEvent is an event the bridge pushes: about document doc, or
/// about the app when doc is 0.
public struct BridgeEvent: Sendable {
    public let name: String
    public let doc: Int
    /// data is the event's data as JSON, "null" when it has none.
    public let data: Data

    public init(name: String, doc: Int, data: Data) {
        self.name = name
        self.doc = doc
        self.data = data
    }

    public func decode<T: Decodable>(_ type: T.Type) throws -> T {
        try JSONDecoder().decode(T.self, from: data)
    }
}

/// Bridge runs mdsh-swift-bridge and talks to it in JSON lines: one
/// request per line on its stdin, and answers and events on its stdout.
/// Calls may be made from any thread. Events are handed to onEvent, and
/// answers resume their callers, on the main queue in the order they
/// came, so a call's answer is never seen before the events sent ahead
/// of it.
public final class Bridge: @unchecked Sendable {
    private let process: Process
    private let stdin: FileHandle
    private let lock = NSLock()
    private var nextID: Int64 = 0
    private var pending: [Int64: (Result<Data, Error>) -> Void] = [:]
    private var buffer = Data()
    private var closed = false

    /// onEvent is called on the main queue for each event.
    public var onEvent: (@Sendable (BridgeEvent) -> Void)?
    /// onExit is called on the main queue if the bridge ends.
    public var onExit: (@Sendable (Int32) -> Void)?
    /// onStderr is called on the main queue with what the bridge logs.
    public var onStderr: (@Sendable (String) -> Void)?

    /// Starts the bridge at executable, with args.
    public init(executable: URL, arguments: [String] = [], environment: [String: String]? = nil) throws {
        // A write to a bridge that has died fails the call with EPIPE
        // rather than killing the app.
        signal(SIGPIPE, SIG_IGN)
        process = Process()
        process.executableURL = executable
        process.arguments = arguments
        if let environment { process.environment = environment }
        let inPipe = Pipe(), outPipe = Pipe(), errPipe = Pipe()
        process.standardInput = inPipe
        process.standardOutput = outPipe
        process.standardError = errPipe
        stdin = inPipe.fileHandleForWriting

        outPipe.fileHandleForReading.readabilityHandler = { [weak self] h in
            let data = h.availableData
            if data.isEmpty {
                h.readabilityHandler = nil
                return
            }
            self?.received(data)
        }
        errPipe.fileHandleForReading.readabilityHandler = { [weak self] h in
            let data = h.availableData
            if data.isEmpty {
                h.readabilityHandler = nil
                return
            }
            if let s = String(data: data, encoding: .utf8), let cb = self?.onStderr {
                DispatchQueue.main.async { cb(s) }
            }
        }
        process.terminationHandler = { [weak self] p in
            self?.failAll(BridgeError("the bridge stopped (status \(p.terminationStatus))"))
            if let cb = self?.onExit {
                let status = p.terminationStatus
                DispatchQueue.main.async { cb(status) }
            }
        }
        try process.run()
    }

    deinit { stop() }

    /// Stops the bridge, which closes every document's shells.
    public func stop() {
        lock.lock()
        let wasClosed = closed
        closed = true
        lock.unlock()
        if !wasClosed {
            try? stdin.close()
        }
    }

    /// Finds the bridge: $MDSH_SWIFT_BRIDGE, the app bundle's
    /// Resources, beside this executable, or on PATH.
    public static func locate() -> URL? {
        let fm = FileManager.default
        let name = "mdsh-swift-bridge"
        var candidates: [URL] = []
        if let env = ProcessInfo.processInfo.environment["MDSH_SWIFT_BRIDGE"], !env.isEmpty {
            candidates.append(URL(fileURLWithPath: env))
        }
        if let res = Bundle.main.resourceURL {
            candidates.append(res.appendingPathComponent(name))
        }
        if let exe = Bundle.main.executableURL {
            candidates.append(exe.deletingLastPathComponent().appendingPathComponent(name))
        }
        let path = ProcessInfo.processInfo.environment["PATH"] ?? ""
        for dir in path.split(separator: ":") {
            candidates.append(URL(fileURLWithPath: String(dir)).appendingPathComponent(name))
        }
        return candidates.first { fm.isExecutableFile(atPath: $0.path) }
    }

    // MARK: Calls

    /// Calls method with params and decodes its result as R.
    public func call<R: Decodable>(_ method: String, _ params: [String: JSONValue] = [:], as: R.Type = R.self) async throws -> R {
        let data = try await raw(method, params)
        return try JSONDecoder().decode(R.self, from: data)
    }

    /// Calls method with params, for a method with no result.
    public func call(_ method: String, _ params: [String: JSONValue] = [:]) async throws {
        _ = try await raw(method, params)
    }

    /// Calls method with params and returns its result as JSON, "null"
    /// when it has none.
    public func raw(_ method: String, _ params: [String: JSONValue]) async throws -> Data {
        let line: Data
        let id: Int64
        lock.lock()
        nextID += 1
        id = nextID
        let isClosed = closed
        lock.unlock()
        if isClosed { throw BridgeError("the bridge is stopped") }
        do {
            struct Request: Encodable {
                let id: Int64
                let method: String
                let params: [String: JSONValue]
            }
            line = try JSONEncoder().encode(Request(id: id, method: method, params: params)) + Data([0x0A])
        }
        return try await withCheckedThrowingContinuation { cont in
            lock.lock()
            pending[id] = { cont.resume(with: $0) }
            lock.unlock()
            do {
                try stdin.write(contentsOf: line)
            } catch {
                lock.lock()
                let f = pending.removeValue(forKey: id)
                lock.unlock()
                f?(.failure(error))
            }
        }
    }

    // MARK: Reading

    private func received(_ data: Data) {
        lock.lock()
        buffer.append(data)
        var lines: [Data] = []
        while let nl = buffer.firstIndex(of: 0x0A) {
            lines.append(buffer[buffer.startIndex..<nl])
            buffer.removeSubrange(buffer.startIndex...nl)
        }
        lock.unlock()
        for l in lines where !l.isEmpty {
            handle(line: Data(l))
        }
    }

    private func handle(line: Data) {
        guard let obj = try? JSONSerialization.jsonObject(with: line, options: [.fragmentsAllowed]) as? [String: Any] else {
            return
        }
        if let name = obj["event"] as? String {
            let doc = (obj["doc"] as? NSNumber)?.intValue ?? 0
            let data = Self.json(obj["data"])
            let ev = BridgeEvent(name: name, doc: doc, data: data)
            if let cb = onEvent {
                DispatchQueue.main.async { cb(ev) }
            }
            return
        }
        guard let id = (obj["id"] as? NSNumber)?.int64Value else { return }
        lock.lock()
        let f = pending.removeValue(forKey: id)
        lock.unlock()
        let result: Result<Data, Error> =
            (obj["error"] as? String).map { .failure(BridgeError($0)) } ?? .success(Self.json(obj["result"]))
        DispatchQueue.main.async { f?(result) }
    }

    private static func json(_ v: Any?) -> Data {
        guard let v, !(v is NSNull) else { return Data("null".utf8) }
        return (try? JSONSerialization.data(withJSONObject: v, options: [.fragmentsAllowed])) ?? Data("null".utf8)
    }

    private func failAll(_ err: Error) {
        lock.lock()
        let all = pending
        pending = [:]
        closed = true
        lock.unlock()
        for f in all.values { f(.failure(err)) }
    }
}

/// Empty decodes any result, for calls whose result is ignored.
public struct Empty: Decodable, Sendable {
    public init() {}
    public init(from decoder: Decoder) throws {}
}
