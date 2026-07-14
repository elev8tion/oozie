// Prints the CGWindowID of the frontmost on-screen window whose owning
// process name contains the given string. Used by visual-review.sh to
// screenshot just the app's window.
import CoreGraphics
import Foundation

guard CommandLine.arguments.count > 1 else {
	FileHandle.standardError.write(Data("usage: WindowID.swift <app name>\n".utf8))
	exit(2)
}
let name = CommandLine.arguments[1]
let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] ?? []
for w in list {
	let owner = w[kCGWindowOwnerName as String] as? String ?? ""
	let layer = w[kCGWindowLayer as String] as? Int ?? -1
	if layer == 0, owner.localizedCaseInsensitiveContains(name),
	   let num = w[kCGWindowNumber as String] as? Int {
		print(num)
		exit(0)
	}
}
exit(1)
