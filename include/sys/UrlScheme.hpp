#pragma once

#include <QString>

// Portable install: the exe path moves, so registration re-runs at startup and is diffed against the settings mirror (url_scheme_mirror).

// Opaque state, revision-prefixed so that adding entries re-registers installs that never moved.
QString UrlScheme_DesiredState();

// Whether the registration still points at this install; the mirror cannot see another install taking the shared entries over.
bool UrlScheme_IsCurrent();

void UrlScheme_Apply();

// Inverse of Apply(): drops only what we wrote.
void UrlScheme_Remove();

// Startup path; skipped entirely while auto registration is off.
void UrlScheme_RegisterIfNeeded();

// Settings buttons; both move the mirror so startup neither redoes nor undoes them, and Uninstall also turns auto registration off.
bool UrlScheme_Install();

void UrlScheme_Uninstall();
