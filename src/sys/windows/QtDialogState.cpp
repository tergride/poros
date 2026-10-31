#include "include/sys/windows/QtDialogState.h"

#include <string>
#include <utility>
#include <vector>

#include <windows.h>
#include <shlwapi.h>

static const wchar_t *const kFileDialogKey = L"Software\\QtProject\\OrganizationDefaults\\FileDialog";

struct QtDialogStateGuard::Saved {
    struct Value {
        std::wstring name;
        DWORD type;
        std::vector<BYTE> data;
    };
    bool existed = false;
    std::vector<Value> values;
};

QtDialogStateGuard::QtDialogStateGuard() : saved(std::make_unique<Saved>()) {
    HKEY key = nullptr;
    if (RegOpenKeyExW(HKEY_CURRENT_USER, kFileDialogKey, 0, KEY_QUERY_VALUE, &key) != ERROR_SUCCESS) return;
    saved->existed = true;

    DWORD count = 0;
    DWORD longestName = 0;
    DWORD longestData = 0;
    RegQueryInfoKeyW(key, nullptr, nullptr, nullptr, nullptr, nullptr, nullptr, &count, &longestName, &longestData, nullptr, nullptr);
    for (DWORD i = 0; i < count; ++i) {
        std::wstring name(longestName + 1, L'\0');
        std::vector<BYTE> data(longestData);
        DWORD nameLength = longestName + 1;
        DWORD dataSize = longestData;
        DWORD type = 0;
        if (RegEnumValueW(key, i, name.data(), &nameLength, nullptr, &type, data.data(), &dataSize) != ERROR_SUCCESS) continue;
        name.resize(nameLength);
        data.resize(dataSize);
        saved->values.push_back({std::move(name), type, std::move(data)});
    }
    RegCloseKey(key);
}

QtDialogStateGuard::~QtDialogStateGuard() {
    RegDeleteTreeW(HKEY_CURRENT_USER, kFileDialogKey);
    if (!saved->existed) {
        // Only empty keys go: whatever another Qt program keeps next to FileDialog stays.
        SHDeleteEmptyKeyW(HKEY_CURRENT_USER, L"Software\\QtProject\\OrganizationDefaults");
        SHDeleteEmptyKeyW(HKEY_CURRENT_USER, L"Software\\QtProject");
        return;
    }
    HKEY key = nullptr;
    if (RegCreateKeyExW(HKEY_CURRENT_USER, kFileDialogKey, 0, nullptr, REG_OPTION_NON_VOLATILE, KEY_SET_VALUE, nullptr, &key, nullptr) != ERROR_SUCCESS) return;
    for (const auto &value : saved->values) {
        RegSetValueExW(key, value.name.c_str(), 0, value.type, value.data.data(), static_cast<DWORD>(value.data.size()));
    }
    RegCloseKey(key);
}
