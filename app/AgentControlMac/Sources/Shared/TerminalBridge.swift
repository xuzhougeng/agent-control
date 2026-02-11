import Foundation
import SwiftTerm

final class TerminalBridge {
    private weak var backingTerminalView: TerminalView?
    private var pendingChunks: [Data] = []
    private var pendingReset = false
    private var pendingScrollToBottom = false
    private var followLatestOutput = true
    private let maxPendingBytes = 512 * 1024
    private let followThreshold = 0.98
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

    func updateScrollPosition(_ position: Double, canScroll: Bool) {
        // When there is no scrollback yet, SwiftTerm reports position=0.
        // Treat that state as "following latest" instead of "scrolled away".
        if !canScroll {
            followLatestOutput = true
            return
        }
        followLatestOutput = position >= followThreshold
    }

    func prepareForAttach() {
        followLatestOutput = true
        pendingScrollToBottom = true
    }

    func feed(_ data: Data) {
        guard !data.isEmpty else { return }
        if terminalView == nil {
            enqueuePending(data)
            if followLatestOutput { pendingScrollToBottom = true }
            return
        }
        let bytes = [UInt8](data)
        if Thread.isMainThread {
            terminalView?.feed(byteArray: bytes[...])
            if followLatestOutput { scrollToBottomNow() }
        } else {
            DispatchQueue.main.async { [weak self] in
                self?.terminalView?.feed(byteArray: bytes[...])
                if self?.followLatestOutput == true { self?.scrollToBottomNow() }
            }
        }
    }

    func clear() {
        if terminalView == nil {
            pendingChunks.removeAll()
            pendingBytes = 0
            pendingReset = true
            pendingScrollToBottom = true
            followLatestOutput = true
            return
        }
        if Thread.isMainThread {
            terminalView?.feed(text: "\u{1b}c")
            requestScrollToBottom()
        } else {
            DispatchQueue.main.async { [weak self] in
                self?.terminalView?.feed(text: "\u{1b}c")
                self?.requestScrollToBottom()
            }
        }
    }

    func requestScrollToBottom() {
        pendingScrollToBottom = true
        followLatestOutput = true
        scrollToBottomNow()
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
        let shouldScrollToBottom = pendingScrollToBottom || (followLatestOutput && !replay.isEmpty)
        pendingChunks.removeAll()
        pendingBytes = 0
        pendingReset = false
        pendingScrollToBottom = false

        let flush = {
            if shouldReset {
                terminalView.feed(text: "\u{1b}c")
            }
            for chunk in replay {
                let bytes = [UInt8](chunk)
                terminalView.feed(byteArray: bytes[...])
            }
            if shouldScrollToBottom {
                self.scrollToBottomNow(on: terminalView)
            }
        }
        if Thread.isMainThread {
            flush()
        } else {
            DispatchQueue.main.async(execute: flush)
        }
    }

    private func scrollToBottomNow(on view: TerminalView? = nil) {
        guard let target = view ?? terminalView else { return }
        target.scroll(toPosition: 1.0)
    }
}
