#include "include/sys/ProcessMetrics.hpp"

#include <QThread>

#include <chrono>

#include <windows.h>
#include <psapi.h>

namespace {
    // One platform read: cumulative CPU time (ns) + resident memory (bytes).
    struct Raw {
        bool ok = false;
        quint64 cpuNs = 0;
        qint64 rss = 0;
    };

    int cpuCount() {
        const int n = QThread::idealThreadCount();
        return n > 0 ? n : 1;
    }

    qint64 monotonicNs() {
        return static_cast<qint64>(std::chrono::duration_cast<std::chrono::nanoseconds>(
            std::chrono::steady_clock::now().time_since_epoch()).count());
    }

    Raw readRaw(qint64 pid) {
        Raw r;
        if (pid <= 0) return r;

        HANDLE h;
        bool ownHandle = false;
        if (static_cast<DWORD>(pid) == GetCurrentProcessId()) {
            h = GetCurrentProcess(); // pseudo-handle, must not be closed
        } else {
            h = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, static_cast<DWORD>(pid));
            if (h == nullptr) return r;
            ownHandle = true;
        }

        FILETIME ftCreate, ftExit, ftKernel, ftUser;
        if (GetProcessTimes(h, &ftCreate, &ftExit, &ftKernel, &ftUser)) {
            ULARGE_INTEGER k, u;
            k.LowPart = ftKernel.dwLowDateTime;
            k.HighPart = ftKernel.dwHighDateTime;
            u.LowPart = ftUser.dwLowDateTime;
            u.HighPart = ftUser.dwHighDateTime;
            // FILETIME counts are in 100-nanosecond units.
            r.cpuNs = (k.QuadPart + u.QuadPart) * 100ULL;
        }

        struct ProcMemCountersEx2 {
            DWORD   cb;
            DWORD   PageFaultCount;
            SIZE_T  PeakWorkingSetSize;
            SIZE_T  WorkingSetSize;
            SIZE_T  QuotaPeakPagedPoolUsage;
            SIZE_T  QuotaPagedPoolUsage;
            SIZE_T  QuotaPeakNonPagedPoolUsage;
            SIZE_T  QuotaNonPagedPoolUsage;
            SIZE_T  PagefileUsage;
            SIZE_T  PeakPagefileUsage;
            SIZE_T  PrivateUsage;
            SIZE_T  PrivateWorkingSetSize;
            ULONG64 SharedCommitUsage;
        };
        ProcMemCountersEx2 ex2{};
        ex2.cb = sizeof(ex2);
        if (GetProcessMemoryInfo(h, reinterpret_cast<PROCESS_MEMORY_COUNTERS*>(&ex2), sizeof(ex2))
            && ex2.PrivateWorkingSetSize > 0) {
            r.rss = static_cast<qint64>(ex2.PrivateWorkingSetSize);
        } else {
            PROCESS_MEMORY_COUNTERS_EX pmcx{};
            pmcx.cb = sizeof(pmcx);
            if (GetProcessMemoryInfo(h, reinterpret_cast<PROCESS_MEMORY_COUNTERS*>(&pmcx), sizeof(pmcx))) {
                r.rss = static_cast<qint64>(pmcx.PrivateUsage);
            }
        }

        r.ok = true;
        if (ownHandle) CloseHandle(h);
        return r;
    }
}

namespace Sys {
    ProcessMetrics::Sample ProcessMetrics::sample(qint64 pid) {
        Sample s;
        const Raw raw = readRaw(pid);
        if (!raw.ok) {
            prior_.remove(pid);
            return s;
        }
        s.ok = true;
        s.rssBytes = raw.rss;

        const qint64 nowNs = monotonicNs();
        if (const auto it = prior_.constFind(pid); it != prior_.constEnd()) {
            const qint64 dWall = nowNs - it->wallNs;
            const qint64 dCpu = static_cast<qint64>(raw.cpuNs) - static_cast<qint64>(it->cpuTimeNs);
            if (dWall > 0 && dCpu >= 0) {
                s.cpuPercent = static_cast<double>(dCpu) / static_cast<double>(dWall)
                               * 100.0 / static_cast<double>(cpuCount());
                s.cpuValid = true;
            }
        }
        prior_.insert(pid, Prior{raw.cpuNs, nowNs});
        return s;
    }
} // namespace Sys
