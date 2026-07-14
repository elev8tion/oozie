// On-device app-icon generation via Apple Intelligence (ImagePlayground).
// Built into a tiny .app by generate-icon.sh — the ImageCreator API only
// serves foreground apps launched through LaunchServices.
//
// argv: <prompt> <absolute-output.png> [style: illustration|animation|sketch]
import AppKit
import Foundation
import ImagePlayground

let args = CommandLine.arguments
guard args.count > 2 else { exit(2) }
let prompt = args[1]
let out = args[2]
let styleName = args.count > 3 ? args[3] : "illustration"

func fail(_ message: String) -> Never {
	try? message.write(toFile: out + ".err", atomically: true, encoding: .utf8)
	exit(1)
}

final class Delegate: NSObject, NSApplicationDelegate {
	func applicationDidFinishLaunching(_ n: Notification) {
		NSApp.activate(ignoringOtherApps: true)
		Task {
			do {
				let creator = try await ImageCreator()
				let style: ImagePlaygroundStyle
				switch styleName {
				case "animation": style = .animation
				case "sketch": style = .sketch
				default: style = .illustration
				}
				let images = creator.images(for: [.text(prompt)], style: style, limit: 1)
				for try await image in images {
					let rep = NSBitmapImageRep(cgImage: image.cgImage)
					guard let png = rep.representation(using: .png, properties: [:]) else {
						fail("could not encode PNG")
					}
					try png.write(to: URL(fileURLWithPath: out))
					exit(0)
				}
				fail("no image produced")
			} catch {
				fail("Apple Intelligence image generation unavailable: \(error)")
			}
		}
	}
}

let app = NSApplication.shared
let delegate = Delegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
