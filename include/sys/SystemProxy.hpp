#pragma once

#include <QString>

// All return what failed (empty on success) and may be called from any thread.

// The first Apply remembers what every connection had, in system-proxy.json in the working directory as well.
QString SystemProxy_Apply(const QString &host, int port);

// Gives every connection still on our proxy what it had before Apply, so a proxy another app set survives and one the
// user had comes back. A local proxy comes back only while something listens on its port.
QString SystemProxy_Clear();

// At start, before any Apply: does the Clear a Poros that died with its proxy set never got to.
QString SystemProxy_Recover();
