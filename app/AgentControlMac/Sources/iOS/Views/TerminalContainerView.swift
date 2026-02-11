import SwiftUI
import SwiftTerm

struct TerminalContainerView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(spacing: 0) {
            SwiftTermView(appState: appState)
                .frame(maxWidth: .infinity, maxHeight: .infinity)

            // Quick-input toolbar for keys hard to reach on iOS keyboard
            TerminalKeybar(appState: appState)

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
            .background(Color(uiColor: .secondarySystemBackground))
        }
    }
}

// MARK: - Quick-input keys (Tab, Esc, Ctrl-C, arrows)

private struct TerminalKeybar: View {
    let appState: AppState

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                keyButton("Esc", bytes: [0x1b])
                keyButton("Tab", bytes: [0x09])
                keyButton("Ctrl-C", bytes: [0x03])
                keyButton("\u{25B2}", bytes: [0x1b, 0x5b, 0x41]) // Up
                keyButton("\u{25BC}", bytes: [0x1b, 0x5b, 0x42]) // Down
                keyButton("\u{25C0}", bytes: [0x1b, 0x5b, 0x44]) // Left
                keyButton("\u{25B6}", bytes: [0x1b, 0x5b, 0x43]) // Right
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
        }
        .background(Color(uiColor: .tertiarySystemBackground))
    }

    private func keyButton(_ label: String, bytes: [UInt8]) -> some View {
        Button(label) {
            appState.sendTerminalInput(Data(bytes))
        }
        .font(.system(size: 13, weight: .medium, design: .monospaced))
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .background(Color(uiColor: .systemFill))
        .cornerRadius(6)
    }
}

// MARK: - UIViewRepresentable

struct SwiftTermView: UIViewRepresentable {
    let appState: AppState

    func makeUIView(context: Context) -> TerminalView {
        let tv = TerminalView(frame: CGRect(x: 0, y: 0, width: 400, height: 300))
        tv.terminalDelegate = context.coordinator
        tv.nativeBackgroundColor = UIColor(red: 0.043, green: 0.063, blue: 0.125, alpha: 1)
        tv.nativeForegroundColor = .white
        appState.terminalBridge.terminalView = tv
        return tv
    }

    func updateUIView(_ uiView: TerminalView, context: Context) {}

    func makeCoordinator() -> Coordinator { Coordinator(appState: appState) }

    final class Coordinator: NSObject, TerminalViewDelegate {
        let appState: AppState
        init(appState: AppState) { self.appState = appState }

        func send(source: TerminalView, data: ArraySlice<UInt8>) {
            let bytes = Data(data)
            DispatchQueue.main.async { [weak self] in
                self?.appState.sendTerminalInput(bytes)
            }
        }

        func scrolled(source: TerminalView, position: Double) {}
        func setTerminalTitle(source: TerminalView, title: String) {}

        func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) {
            DispatchQueue.main.async { [weak self] in
                guard let self else { return }
                appState.terminalBridge.currentCols = newCols
                appState.terminalBridge.currentRows = newRows
                appState.sendResize(cols: newCols, rows: newRows)
            }
        }

        func clipboardCopy(source: TerminalView, content: Data) {
            if let str = String(data: content, encoding: .utf8) {
                UIPasteboard.general.string = str
            }
        }

        func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
        func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
        func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {
            if let url = URL(string: link) {
                UIApplication.shared.open(url)
            }
        }
    }
}
