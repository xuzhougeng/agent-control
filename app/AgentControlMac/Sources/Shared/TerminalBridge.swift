import Foundation
import SwiftTerm

final class TerminalBridge {
    weak var terminalView: TerminalView?
    var currentCols: Int = 120
    var currentRows: Int = 30

    func feed(_ data: Data) {
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
        if Thread.isMainThread {
            terminalView?.feed(text: "\u{1b}c")
        } else {
            DispatchQueue.main.async { [weak self] in
                self?.terminalView?.feed(text: "\u{1b}c")
            }
        }
    }
}
