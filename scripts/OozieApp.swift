// oozie native launcher: starts the embedded oozie-server on a free
// localhost port and hosts the UI in a native window (WKWebView).
// Quitting the app terminates the server, which reaps pi subprocesses.
import AppKit
import WebKit

final class AppDelegate: NSObject, NSApplicationDelegate, WKUIDelegate {
	var window: NSWindow!
	var webView: WKWebView!
	var server: Process?
	var port: UInt16 = 0

	func applicationDidFinishLaunching(_ notification: Notification) {
		port = Self.freePort()
		launchServer()
		buildMenu()
		buildWindow()
		loadWhenReady(attemptsLeft: 100)
	}

	func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

	func applicationWillTerminate(_ notification: Notification) {
		server?.terminate() // SIGTERM → graceful shutdown, pi agents reaped
		server?.waitUntilExit()
	}

	// MARK: server

	func launchServer() {
		let proc = Process()
		proc.executableURL = URL(fileURLWithPath: Bundle.main.bundlePath + "/Contents/MacOS/oozie-server")
		var env = ProcessInfo.processInfo.environment
		env["ADDR"] = "127.0.0.1:\(port)"
		env["OOZIE_PARENT_WATCH"] = "1"
		env.removeValue(forKey: "OOZIE_OPEN_BROWSER")
		proc.environment = env
		// The server watches this pipe: if we die (even force-quit), the
		// pipe closes and the server shuts itself down.
		proc.standardInput = Pipe()
		do {
			try proc.run()
			server = proc
		} catch {
			fatalAlert("oozie could not start its server: \(error.localizedDescription)")
		}
	}

	static func freePort() -> UInt16 {
		let sock = socket(AF_INET, SOCK_STREAM, 0)
		defer { close(sock) }
		var addr = sockaddr_in()
		addr.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
		addr.sin_family = sa_family_t(AF_INET)
		addr.sin_port = 0
		addr.sin_addr.s_addr = inet_addr("127.0.0.1")
		var bound = addr
		let ok = withUnsafePointer(to: &bound) {
			$0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
				Darwin.bind(sock, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
			}
		}
		guard ok == 0 else { return 8080 }
		var out = sockaddr_in()
		var len = socklen_t(MemoryLayout<sockaddr_in>.size)
		withUnsafeMutablePointer(to: &out) {
			$0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
				_ = getsockname(sock, $0, &len)
			}
		}
		return UInt16(bigEndian: out.sin_port)
	}

	// MARK: window

	func buildWindow() {
		let config = WKWebViewConfiguration()
		webView = WKWebView(frame: .zero, configuration: config)
		webView.uiDelegate = self
		webView.allowsMagnification = true

		window = NSWindow(
			contentRect: NSRect(x: 0, y: 0, width: 1280, height: 840),
			styleMask: [.titled, .closable, .miniaturizable, .resizable],
			backing: .buffered, defer: false)
		window.title = "oozie"
		window.minSize = NSSize(width: 760, height: 500)
		window.contentView = webView
		window.center()
		window.setFrameAutosaveName("oozie-main")
		window.makeKeyAndOrderFront(nil)
		NSApp.activate(ignoringOtherApps: true)
	}

	func loadWhenReady(attemptsLeft: Int) {
		guard attemptsLeft > 0 else {
			fatalAlert("oozie's server did not become ready.")
			return
		}
		let url = URL(string: "http://127.0.0.1:\(port)/projects")!
		let task = URLSession.shared.dataTask(with: url) { _, response, _ in
			DispatchQueue.main.async {
				if let http = response as? HTTPURLResponse, http.statusCode < 500 {
					self.webView.load(URLRequest(url: URL(string: "http://127.0.0.1:\(self.port)/")!))
				} else {
					DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
						self.loadWhenReady(attemptsLeft: attemptsLeft - 1)
					}
				}
			}
		}
		task.resume()
	}

	// MARK: JS dialogs (hx-confirm and friends)

	func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String,
	             initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
		let alert = NSAlert()
		alert.messageText = message
		alert.addButton(withTitle: "OK")
		alert.runModal()
		completionHandler()
	}

	func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String,
	             initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
		let alert = NSAlert()
		alert.messageText = message
		alert.addButton(withTitle: "OK")
		alert.addButton(withTitle: "Cancel")
		completionHandler(alert.runModal() == .alertFirstButtonReturn)
	}

	func webView(_ webView: WKWebView, runJavaScriptTextInputPanelWithPrompt prompt: String,
	             defaultText: String?, initiatedByFrame frame: WKFrameInfo,
	             completionHandler: @escaping (String?) -> Void) {
		let alert = NSAlert()
		alert.messageText = prompt
		let field = NSTextField(frame: NSRect(x: 0, y: 0, width: 260, height: 24))
		field.stringValue = defaultText ?? ""
		alert.accessoryView = field
		alert.addButton(withTitle: "OK")
		alert.addButton(withTitle: "Cancel")
		completionHandler(alert.runModal() == .alertFirstButtonReturn ? field.stringValue : nil)
	}

	// MARK: menu

	func buildMenu() {
		let main = NSMenu()

		let appItem = NSMenuItem()
		main.addItem(appItem)
		let appMenu = NSMenu()
		appItem.submenu = appMenu
		appMenu.addItem(withTitle: "About oozie", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
		appMenu.addItem(.separator())
		appMenu.addItem(withTitle: "Hide oozie", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
		appMenu.addItem(.separator())
		appMenu.addItem(withTitle: "Quit oozie", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")

		let editItem = NSMenuItem()
		main.addItem(editItem)
		let edit = NSMenu(title: "Edit")
		editItem.submenu = edit
		edit.addItem(withTitle: "Undo", action: Selector(("undo:")), keyEquivalent: "z")
		edit.addItem(withTitle: "Redo", action: Selector(("redo:")), keyEquivalent: "Z")
		edit.addItem(.separator())
		edit.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
		edit.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
		edit.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
		edit.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")

		let viewItem = NSMenuItem()
		main.addItem(viewItem)
		let view = NSMenu(title: "View")
		viewItem.submenu = view
		let reload = NSMenuItem(title: "Reload", action: #selector(reloadPage), keyEquivalent: "r")
		reload.target = self
		view.addItem(reload)

		NSApp.mainMenu = main
	}

	@objc func reloadPage() { webView.reload() }

	func fatalAlert(_ message: String) {
		let alert = NSAlert()
		alert.messageText = message
		alert.runModal()
		NSApp.terminate(nil)
	}
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
