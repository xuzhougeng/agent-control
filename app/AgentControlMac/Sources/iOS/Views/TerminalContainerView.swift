import SwiftUI
import SwiftTerm

struct TerminalContainerView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        VStack(spacing: 0) {
            SwiftTermView(appState: appState)
                .frame(maxWidth: .infinity, maxHeight: .infinity)

            // Keybar visible when keyboard is hidden; keyboard's inputAccessoryView takes over when keyboard is up
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

// MARK: - Quick-input keys (two rows)

private struct TerminalKeybar: View {
    let appState: AppState

    var body: some View {
        VStack(spacing: 8) {
            HStack(spacing: 8) {
                dismissButton
                keyButton("/", bytes: [0x2f])
                keyButton("Esc", bytes: [0x1b])
                keyButton("Tab", bytes: [0x09])
                keyButton("Ctrl-C", bytes: [0x03])
            }
            HStack(spacing: 8) {
                keyButton("Enter", bytes: [0x0d])
                keyButton("\u{25B2}", bytes: [0x1b, 0x5b, 0x41]) // Up
                keyButton("\u{25BC}", bytes: [0x1b, 0x5b, 0x42]) // Down
                keyButton("\u{25C0}", bytes: [0x1b, 0x5b, 0x44]) // Left
                keyButton("\u{25B6}", bytes: [0x1b, 0x5b, 0x43]) // Right
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(Color(uiColor: .tertiarySystemBackground))
    }

    private var dismissButton: some View {
        Button {
            dismissKeyboard()
        } label: {
            Image(systemName: "keyboard.chevron.compact.down")
                .font(.system(size: 15, weight: .semibold))
                .frame(minWidth: 28)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .background(Color(uiColor: .systemFill))
        .cornerRadius(6)
    }

    private func dismissKeyboard() {
        if let terminalView = appState.terminalBridge.terminalView {
            _ = terminalView.resignFirstResponder()
            terminalView.window?.endEditing(true)
        }
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
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

// MARK: - UIKit inputAccessoryView keybar (shown above keyboard)

private final class KeybarAccessoryView: UIView {
    private weak var appState: AppState?
    weak var terminalView: TerminalView?
    private static let accessoryHeight: CGFloat = 84

    init(appState: AppState) {
        self.appState = appState
        super.init(frame: CGRect(x: 0, y: 0, width: UIScreen.main.bounds.width, height: Self.accessoryHeight))
        autoresizingMask = .flexibleWidth
        backgroundColor = .tertiarySystemBackground
        setupButtons()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError() }

    override var intrinsicContentSize: CGSize {
        CGSize(width: UIView.noIntrinsicMetric, height: Self.accessoryHeight)
    }

    private func setupButtons() {
        let firstRow = UIStackView()
        firstRow.axis = .horizontal
        firstRow.spacing = 8
        firstRow.translatesAutoresizingMaskIntoConstraints = false

        let secondRow = UIStackView()
        secondRow.axis = .horizontal
        secondRow.spacing = 8
        secondRow.translatesAutoresizingMaskIntoConstraints = false

        var dismissCfg = UIButton.Configuration.filled()
        dismissCfg.baseBackgroundColor = .systemFill
        dismissCfg.baseForegroundColor = .label
        dismissCfg.contentInsets = NSDirectionalEdgeInsets(top: 6, leading: 10, bottom: 6, trailing: 10)
        dismissCfg.cornerStyle = .medium
        dismissCfg.image = UIImage(systemName: "keyboard.chevron.compact.down")
        let dismissButton = UIButton(configuration: dismissCfg, primaryAction: UIAction { [weak self] _ in
            _ = self?.terminalView?.resignFirstResponder()
            self?.terminalView?.window?.endEditing(true)
            self?.window?.endEditing(true)
            UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
        })
        firstRow.addArrangedSubview(dismissButton)
        firstRow.addArrangedSubview(makeKeyButton("/", bytes: [0x2f]))
        firstRow.addArrangedSubview(makeKeyButton("Esc", bytes: [0x1b]))
        firstRow.addArrangedSubview(makeKeyButton("Tab", bytes: [0x09]))
        firstRow.addArrangedSubview(makeKeyButton("Ctrl-C", bytes: [0x03]))

        secondRow.addArrangedSubview(makeKeyButton("Enter", bytes: [0x0d]))
        secondRow.addArrangedSubview(makeKeyButton("\u{25B2}", bytes: [0x1b, 0x5b, 0x41]))
        secondRow.addArrangedSubview(makeKeyButton("\u{25BC}", bytes: [0x1b, 0x5b, 0x42]))
        secondRow.addArrangedSubview(makeKeyButton("\u{25C0}", bytes: [0x1b, 0x5b, 0x44]))
        secondRow.addArrangedSubview(makeKeyButton("\u{25B6}", bytes: [0x1b, 0x5b, 0x43]))

        let column = UIStackView(arrangedSubviews: [firstRow, secondRow])
        column.axis = .vertical
        column.spacing = 8
        column.translatesAutoresizingMaskIntoConstraints = false
        addSubview(column)

        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 10),
            column.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor, constant: -10),
            column.topAnchor.constraint(equalTo: topAnchor, constant: 6),
            column.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -6),
        ])
    }

    private func makeKeyButton(_ label: String, bytes: [UInt8]) -> UIButton {
        var cfg = UIButton.Configuration.filled()
        cfg.baseBackgroundColor = .systemFill
        cfg.baseForegroundColor = .label
        cfg.contentInsets = NSDirectionalEdgeInsets(top: 6, leading: 12, bottom: 6, trailing: 12)
        cfg.cornerStyle = .medium
        cfg.title = label
        cfg.titleTextAttributesTransformer = .init { attr in
            var a = attr
            a.font = UIFont.monospacedSystemFont(ofSize: 13, weight: .medium)
            return a
        }
        return UIButton(configuration: cfg, primaryAction: UIAction { [weak self] _ in
            self?.appState?.sendTerminalInput(Data(bytes))
        })
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
        let accessory = KeybarAccessoryView(appState: appState)
        accessory.terminalView = tv
        tv.inputAccessoryView = accessory
        appState.terminalBridge.terminalView = tv
        DispatchQueue.main.async {
            context.coordinator.syncTerminalSize(from: tv)
        }
        return tv
    }

    func updateUIView(_ uiView: TerminalView, context: Context) {
        context.coordinator.syncTerminalSize(from: uiView)
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
