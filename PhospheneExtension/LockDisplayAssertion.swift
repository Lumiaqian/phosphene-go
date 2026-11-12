import Darwin
import Foundation
import IOKit.ps
import IOKit.pwr_mgt

/// Confined to the playlist queue. An assertion automatically dies with this process.
final class LockDisplayAssertion {
    private var assertion: IOPMAssertionID = 0
    private var lockToken: Int32 = -1
    private var lastCheck = Date.distantPast
    private var lastRequest = false
    private var lastLocked = false
    var isActive: Bool {
        assertion != 0
    }

    init() {
        notify_register_check("com.apple.sessionagent.screenIsLocked", &lockToken)
    }
    func update(enabled: Bool, locked: Bool, active: Bool, asleep: Bool) {
        let request = enabled && active && !asleep
        // Polling also covers power changes and a missed unlock notification.
        guard request != lastRequest || Date().timeIntervalSince(lastCheck) >= 1 || locked != lastLocked else { return }
        lastCheck = Date(); lastRequest = request; lastLocked = locked
        var lockState: UInt64 = 0
        let lockedNow = notify_get_state(lockToken, &lockState) == NOTIFY_STATUS_OK ? lockState != 0 : locked
        guard request, lockedNow,
              let info = IOPSCopyPowerSourcesInfo()?.takeRetainedValue(),
              let source = IOPSGetProvidingPowerSourceType(info)?.takeUnretainedValue(),
              source as String == kIOPSACPowerValue else { release(); return }
        if assertion == 0 {
            let result = IOPMAssertionCreateWithName(
                kIOPMAssertionTypePreventUserIdleDisplaySleep as CFString,
                IOPMAssertionLevel(kIOPMAssertionLevelOn),
                "Phosphene Go 接电锁屏播放" as CFString,
                &assertion,
            )
            if result != kIOReturnSuccess {
                assertion = 0; extensionLog("[Lock] Assertion failed: \(result)")
            }
        }
    }
    private func release() {
        if assertion != 0 {
            IOPMAssertionRelease(assertion); assertion = 0
        }
    }
    deinit {
        release(); if lockToken >= 0 {
            notify_cancel(lockToken)
        }
    }
}
