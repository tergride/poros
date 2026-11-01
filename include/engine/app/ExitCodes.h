#pragma once

// Process exit codes of -dump-configs.
namespace Engine::ExitCode {
    inline constexpr int Ok = 0;
    inline constexpr int DataDirOwned = 3;
    inline constexpr int Usage = 64;
    inline constexpr int NoInput = 66;
    inline constexpr int CantCreate = 73;
    inline constexpr int IoError = 74;
} // namespace Engine::ExitCode
