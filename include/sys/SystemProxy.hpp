#pragma once

#include <QString>

// Both return what failed (empty on success) and may be called from any thread.

QString SystemProxy_Apply(const QString &host, int port);

// Undoes SystemProxy_Apply(); does nothing unless this process applied a proxy and it is still in place, so proxies set by other apps survive.
QString SystemProxy_Clear();
