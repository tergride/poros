#pragma once

#include <optional>

#include <QJsonObject>
#include <QStringList>

#include "include/engine/services/ServiceTypes.h"

namespace Engine::SettingsRegistry {
    struct Context {
        bool profileRunning = false;
        bool tunOn = false;
    };

    struct PutResult {
        // Setting names, in request order.
        QStringList changed;
        // A changed key takes effect only when the running profile restarts.
        bool restartNeeded = false;
    };

    // Main thread. Checks every key before writing any; then writes the changed ones, saves once, applies them and emits
    // SettingsChanged. Nothing is written when it returns an error.
    std::optional<ServiceError> Put(const QJsonObject &patch, const Context &context, PutResult *result);
} // namespace Engine::SettingsRegistry
