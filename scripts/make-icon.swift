// Draws oozie's app icon (rounded square, blue gradient, "oz" glyph)
// and writes it as a PNG to the path given as the first argument.
import AppKit

let size = NSSize(width: 1024, height: 1024)
let image = NSImage(size: size)
image.lockFocus()

let rect = NSRect(x: 64, y: 64, width: 896, height: 896)
let path = NSBezierPath(roundedRect: rect, xRadius: 200, yRadius: 200)
NSGradient(
	starting: NSColor(calibratedRed: 0.48, green: 0.64, blue: 1.0, alpha: 1),
	ending: NSColor(calibratedRed: 0.12, green: 0.32, blue: 0.82, alpha: 1)
)!.draw(in: path, angle: -90)

let paragraph = NSMutableParagraphStyle()
paragraph.alignment = .center
let attrs: [NSAttributedString.Key: Any] = [
	.font: NSFont.systemFont(ofSize: 440, weight: .heavy),
	.foregroundColor: NSColor.white,
	.paragraphStyle: paragraph,
]
("oz" as NSString).draw(in: NSRect(x: 0, y: 250, width: 1024, height: 540), withAttributes: attrs)

image.unlockFocus()

guard let tiff = image.tiffRepresentation,
      let rep = NSBitmapImageRep(data: tiff),
      let png = rep.representation(using: .png, properties: [:]) else {
	fputs("could not render icon\n", stderr)
	exit(1)
}
try! png.write(to: URL(fileURLWithPath: CommandLine.arguments[1]))
