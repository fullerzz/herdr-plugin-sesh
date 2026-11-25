// Use the entire guest display at 1x so capture and pointer coordinates agree.
import CoreGraphics
let display = CGMainDisplayID()
let modes = CGDisplayCopyAllDisplayModes(display, nil) as! [CGDisplayMode]
guard let mode = modes.first(where: { $0.width == 2200 && $0.height == 1200 }) else {
    fatalError("The demo needs a 2200x1200 Lume display")
}
var config: CGDisplayConfigRef?
precondition(CGBeginDisplayConfiguration(&config) == .success)
precondition(CGConfigureDisplayWithDisplayMode(config, display, mode, nil) == .success)
precondition(CGCompleteDisplayConfiguration(config, .permanently) == .success)
