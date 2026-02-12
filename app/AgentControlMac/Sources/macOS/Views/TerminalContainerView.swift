import SwiftUI
import SwiftTerm

struct TerminalContainerView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(spacing: 0) {
            SwiftTermView(appState: appState)
                .frame(maxWidth: .infinity, maxHeight: .infinity)

            HStack(spacing: 8) {
                Text(appState.selectedSessionID.map { "Session: \(String($0.prefix(8)))" } ?? "Session: (none)")
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                Spacer()
                Text("\(appState.terminalBridge.currentCols)×\(appState.terminalBridge.currentRows)")
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundColor(.secondary)
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(Color(nsColor: .controlBackgroundColor))
        }
    }
}

// MARK: - NSViewRepresentable

struct SwiftTermView: NSViewRepresentable {
    let appState: AppState

    func makeNSView(context: Context) -> TerminalView {
        let tv = TerminalView(frame: NSRect(x: 0, y: 0, width: 800, height: 400))
        tv.terminalDelegate = context.coordinator
        tv.nativeBackgroundColor = NSColor(red: 0.043, green: 0.063, blue: 0.125, alpha: 1)
        tv.nativeForegroundColor = .white
        appState.terminalBridge.terminalView = tv
        DispatchQueue.main.async {
            context.coordinator.syncTerminalSize(from: tv)
        }
        return tv
    }

    func updateNSView(_ nsView: TerminalView, context: Context) {
        context.coordinator.syncTerminalSize(from: nsView)
    }

    func makeCoordinator() -> Coordinator { Coordinator(appState: appState) }

    final class Coordinator: NSObject, TerminalViewDelegate {
        let appState: AppState
        private var lastSyncedSize: CGSize?

        init(appState: AppState) { self.appState = appState }

        private func applyTerminalSize(cols: Int, rows: Int) {
            guard cols > 0, rows > 0 else { return }
            DispatchQueue.main.async { [weak self] in
                guard let self else { return }
                let size = CGSize(width: cols, height: rows)
                guard self.lastSyncedSize != size else { return }
                self.lastSyncedSize = size
                self.appState.terminalBridge.currentCols = cols
                self.appState.terminalBridge.currentRows = rows
                self.appState.sendResize(cols: cols, rows: rows)
            }
        }

        func syncTerminalSize(from source: TerminalView) {
            let terminal = source.getTerminal()
            applyTerminalSize(cols: terminal.cols, rows: terminal.rows)
        }

        func send(source: TerminalView, data: ArraySlice<UInt8>) {
            let bytes = Data(data)
            DispatchQueue.main.async { [weak self] in
                self?.appState.sendTerminalInput(bytes)
            }
        }

        func scrolled(source: TerminalView, position: Double) {
            DispatchQueue.main.async { [weak self] in
                self?.appState.terminalBridge.updateScrollPosition(position, canScroll: source.canScroll)
            }
        }
        func setTerminalTitle(source: TerminalView, title: String) {}

        func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) {
            applyTerminalSize(cols: newCols, rows: newRows)
        }

        func clipboardCopy(source: TerminalView, content: Data) {
            NSPasteboard.general.clearContents()
            if let str = String(data: content, encoding: .utf8) {
                NSPasteboard.general.setString(str, forType: .string)
            }
        }

        func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
        func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
        func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {
            if let url = URL(string: link) {
                NSWorkspace.shared.open(url)
            }
        }
    }
}
