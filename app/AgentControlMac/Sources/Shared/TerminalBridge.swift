import Foundation
import SwiftTerm

final class TerminalBridge {
    private weak var backingTerminalView: TerminalView?
    private var pendingChunks: [Data] = []
    private var pendingReset = false
    private let maxPendingBytes = 512 * 1024
    private var pendingBytes = 0

    weak var terminalView: TerminalView? {
        get { backingTerminalView }
        set {
            backingTerminalView = newValue
            flushPendingIfPossible()
        }
    }
    var currentCols: Int = 120
    var currentRows: Int = 30

    func feed(_ data: Data) {
        guard !data.isEmpty else { return }
        if terminalView == nil {
            enqueuePending(data)
            return
        }
        let bytes = [UInt8](data)
        if Thread.isMainThread {
            terminalView?.feed(byteArray: bytes[...])
        } else {
            DispatchQueue.main.async { [weak self] in
                self?.terminalView?.feed(byteArray: bytes[...])
            }
        }
    }

    func clear() {
        if terminalView == nil {
            pendingChunks.removeAll()
            pendingBytes = 0
            pendingReset = true
            return
        }
        if Thread.isMainThread {
            terminalView?.feed(text: "\u{1b}c")
        } else {
            DispatchQueue.main.async { [weak self] in
                self?.terminalView?.feed(text: "\u{1b}c")
            }
        }
    }

    private func enqueuePending(_ data: Data) {
        pendingChunks.append(data)
        pendingBytes += data.count
        if pendingBytes <= maxPendingBytes { return }

        while pendingBytes > maxPendingBytes, !pendingChunks.isEmpty {
            let removed = pendingChunks.removeFirst()
            pendingBytes -= removed.count
        }
    }

    private func flushPendingIfPossible() {
        guard let terminalView else { return }
        let replay = pendingChunks
        let shouldReset = pendingReset
        pendingChunks.removeAll()
        pendingBytes = 0
        pendingReset = false

        let flush = {
            if shouldReset {
                terminalView.feed(text: "\u{1b}c")
            }
            for chunk in replay {
                let bytes = [UInt8](chunk)
                terminalView.feed(byteArray: bytes[...])
            }
        }
        if Thread.isMainThread {
            flush()
        } else {
            DispatchQueue.main.async(execute: flush)
        }
    }
}
