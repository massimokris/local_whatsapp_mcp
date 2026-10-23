import AppKit

// Original vector artwork. The icon is generated at every required size; no external assets.
let destination = CommandLine.arguments[1]
try FileManager.default.createDirectory(atPath: destination, withIntermediateDirectories: true)
func drawIcon(size: Int, filename: String) throws {
    let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
    let scale = CGFloat(size) / 1024
    let transform = NSAffineTransform()
    transform.scale(by: scale)
    transform.concat()

    NSColor(calibratedRed: 0.94, green: 0.95, blue: 0.92, alpha: 1).setFill()
    NSBezierPath(roundedRect: NSRect(x: 48, y: 48, width: 928, height: 928), xRadius: 205, yRadius: 205).fill()
    NSColor(calibratedRed: 0.12, green: 0.23, blue: 0.20, alpha: 1).setFill()
    let bubble = NSBezierPath(roundedRect: NSRect(x: 208, y: 278, width: 608, height: 496), xRadius: 150, yRadius: 150)
    bubble.fill()
    let tail = NSBezierPath()
    tail.move(to: NSPoint(x: 271, y: 365))
    tail.line(to: NSPoint(x: 235, y: 205))
    tail.line(to: NSPoint(x: 436, y: 304))
    tail.close(); tail.fill()

    NSColor(calibratedRed: 0.80, green: 0.94, blue: 0.85, alpha: 1).setStroke()
    let connectors = NSBezierPath()
    connectors.move(to: NSPoint(x: 373, y: 518))
    connectors.line(to: NSPoint(x: 512, y: 615))
    connectors.line(to: NSPoint(x: 651, y: 518))
    connectors.line(to: NSPoint(x: 512, y: 421))
    connectors.line(to: NSPoint(x: 373, y: 518))
    connectors.lineWidth = 26
    connectors.lineJoinStyle = .round
    connectors.stroke()
    NSColor(calibratedRed: 0.90, green: 0.98, blue: 0.92, alpha: 1).setFill()
    for (x, y) in [(373, 518), (512, 615), (651, 518), (512, 421)] {
        NSBezierPath(ovalIn: NSRect(x: x - 38, y: y - 38, width: 76, height: 76)).fill()
    }
    NSGraphicsContext.restoreGraphicsState()
    try bitmap.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: destination).appendingPathComponent(filename))
}
for base in [16, 32, 128, 256, 512] {
    try drawIcon(size: base, filename: "icon_\(base)x\(base).png")
    try drawIcon(size: base * 2, filename: "icon_\(base)x\(base)@2x.png")
}
