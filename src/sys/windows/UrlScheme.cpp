#include "include/sys/UrlScheme.hpp"

#include <QApplication>
#include <QDir>
#include <QFileInfo>
#include <QSettings>

#include <string>

#include <shlobj.h>

// In QSettings NativeFormat the value name "Default" is a key's unnamed (Default) value, and '/' separates subkeys.

static const QString kClasses = "HKEY_CURRENT_USER\\Software\\Classes";
static const QString kProgId = "Poros.Config";

static const QStringList kConfigExtensions = {".json", ".conf", ".yaml", ".yml"};
// Claimed before 1.3; never registered again, only taken back.
static const QStringList kRetiredExtensions = {".ini", ".txt"};

static QString openCommand() {
    return "\"" + QDir::toNativeSeparators(QApplication::applicationFilePath()) + "\" \"%1\"";
}

static QString exeName() {
    return QFileInfo(QApplication::applicationFilePath()).fileName();
}

// None of these is keyed by install path, so two portable copies write the same keys and the last launched one wins.
static QStringList commandKeys(Association a) {
    if (a == Association::Links) return {kClasses + "\\poros"};
    return {kClasses + "\\" + kProgId, kClasses + "\\Applications\\" + exeName()};
}

// Every copy is portable: nothing is registered until the user asks.
bool UrlScheme_AutoRegisterByDefault() {
    return false;
}

QString UrlScheme_DesiredState(Association a) {
    return (a == Association::Links ? "v2|" : "v1|") + openCommand();
}

// QSettings creates the key it opens, so checking through it would leave empty keys behind.
static QString registeredCommand(const QString &key) {
    const QString subKey = key.mid(QStringLiteral("HKEY_CURRENT_USER\\").size()) + "\\shell\\open\\command";
    const auto path = reinterpret_cast<LPCWSTR>(subKey.utf16());
    DWORD size = 0;
    if (RegGetValueW(HKEY_CURRENT_USER, path, nullptr, RRF_RT_REG_SZ, nullptr, nullptr, &size) != ERROR_SUCCESS) return {};
    std::wstring value(size / sizeof(wchar_t), L'\0');
    if (RegGetValueW(HKEY_CURRENT_USER, path, nullptr, RRF_RT_REG_SZ, nullptr, value.data(), &size) != ERROR_SUCCESS) return {};
    return QString::fromWCharArray(value.c_str());
}

bool UrlScheme_IsCurrent(Association a) {
    const QString command = openCommand();
    for (const QString &key : commandKeys(a)) {
        if (registeredCommand(key) != command) return false;
    }
    return true;
}

static void applyLinks(const QString &command) {
    QSettings scheme(kClasses + "\\poros", QSettings::NativeFormat);
    scheme.setValue("Default", "URL:Poros Protocol");
    scheme.setValue("URL Protocol", "");
    scheme.setValue("shell/open/command/Default", command);
}

static void applyConfigFiles(const QString &command) {
    QSettings progId(kClasses + "\\" + kProgId, QSettings::NativeFormat);
    progId.setValue("Default", "Poros profile");
    progId.setValue("DefaultIcon/Default", QDir::toNativeSeparators(QApplication::applicationFilePath()) + ",0");
    progId.setValue("shell/open/command/Default", command);

    // OpenWithProgids is the additive half of an association: the extension's own default is left alone.
    for (const QString &ext : kConfigExtensions) {
        QSettings(kClasses + "\\" + ext + "\\OpenWithProgids", QSettings::NativeFormat).setValue(kProgId, "");
    }
    for (const QString &ext : kRetiredExtensions) {
        QSettings(kClasses + "\\" + ext + "\\OpenWithProgids", QSettings::NativeFormat).remove(kProgId);
    }

    // Applications\<exe> is what "Open with > Choose another app" reads, the only route for an extensionless file.
    QSettings app(kClasses + "\\Applications\\" + exeName(), QSettings::NativeFormat);
    app.setValue("FriendlyAppName", "Poros");
    app.setValue("shell/open/command/Default", command);
    app.remove("SupportedTypes");
    for (const QString &ext : kConfigExtensions) {
        app.setValue("SupportedTypes/" + ext, "");
    }
}

void UrlScheme_Apply(Association a) {
    if (a == Association::Links) applyLinks(openCommand());
    else applyConfigFiles(openCommand());

    // QSettings flushes on destruction, so the writers must have returned before the shell is told to reload.
    SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
}

void UrlScheme_Remove(Association a) {
    // Removing from the parent key drops the whole subtree; QSettings::remove("") would only empty it and leave the node behind.
    QSettings classes(kClasses, QSettings::NativeFormat);
    if (a == Association::Links) {
        classes.remove("poros");
    } else {
        classes.remove(kProgId);
        classes.remove("Applications/" + exeName());
        // Only our own progid goes; the extension's default was never ours to touch.
        for (const QString &ext : kConfigExtensions + kRetiredExtensions) {
            QSettings(kClasses + "\\" + ext + "\\OpenWithProgids", QSettings::NativeFormat).remove(kProgId);
        }
    }
    classes.sync();

    SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
}
