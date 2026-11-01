#include "include/sys/UrlScheme.hpp"

#include <QApplication>
#include <QDir>
#include <QSettings>

#include <string>

#include <shlobj.h>

// In QSettings NativeFormat the value name "Default" is a key's unnamed (Default) value, and '/' separates subkeys.

static const QString kClasses = "HKEY_CURRENT_USER\\Software\\Classes";

static QString openCommand() {
    return "\"" + QDir::toNativeSeparators(QApplication::applicationFilePath()) + "\" \"%1\"";
}

QString UrlScheme_DesiredState() {
    return "v2|" + openCommand();
}

// QSettings creates the key it opens, so checking through it would leave empty keys behind.
// Not keyed by install path: two portable copies write the same key and the last launched one wins.
bool UrlScheme_IsCurrent() {
    const QString subKey = QStringLiteral("Software\\Classes\\poros\\shell\\open\\command");
    const auto path = reinterpret_cast<LPCWSTR>(subKey.utf16());
    DWORD size = 0;
    if (RegGetValueW(HKEY_CURRENT_USER, path, nullptr, RRF_RT_REG_SZ, nullptr, nullptr, &size) != ERROR_SUCCESS) return false;
    std::wstring value(size / sizeof(wchar_t), L'\0');
    if (RegGetValueW(HKEY_CURRENT_USER, path, nullptr, RRF_RT_REG_SZ, nullptr, value.data(), &size) != ERROR_SUCCESS) return false;
    return QString::fromWCharArray(value.c_str()) == openCommand();
}

void UrlScheme_Apply() {
    {
        QSettings scheme(kClasses + "\\poros", QSettings::NativeFormat);
        scheme.setValue("Default", "URL:Poros Protocol");
        scheme.setValue("URL Protocol", "");
        scheme.setValue("shell/open/command/Default", openCommand());
    }

    // QSettings flushes on destruction, so the writer must be gone before the shell is told to reload.
    SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
}

void UrlScheme_Remove() {
    // Removing from the parent key drops the whole subtree; QSettings::remove("") would only empty it and leave the node behind.
    QSettings classes(kClasses, QSettings::NativeFormat);
    classes.remove("poros");
    classes.sync();

    SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
}
