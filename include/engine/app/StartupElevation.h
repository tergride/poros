#pragma once

namespace Engine {
    struct StartupElevationInput {
        bool lastRunElevated = false;
        bool elevationDisabled = false;
        bool rememberEnabled = false;
        bool rememberTun = false;
        bool killSwitch = false;
    };

    // A launch only asks for rights that have to work before the first click: everything else asks when it is needed.
    inline bool NeedsElevationAtStartup(const StartupElevationInput &in) {
        if (!in.lastRunElevated || in.elevationDisabled) return false;
        return (in.rememberEnabled && in.rememberTun) || in.killSwitch;
    }
} // namespace Engine
