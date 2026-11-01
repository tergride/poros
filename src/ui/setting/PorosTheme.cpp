#include "include/ui/setting/PorosTheme.h"

#include <QFile>
#include <QHash>
#include <QIconEngine>
#include <QPainter>
#include <QPixmap>
#include <QSvgRenderer>

namespace {
    QByteArray porosThemeIconSource(const QString &name) {
        static QHash<QString, QByteArray> sources;
        const auto it = sources.constFind(name);
        if (it != sources.constEnd()) return *it;
        QFile file(QStringLiteral(":/poros/icons/") + name + QStringLiteral(".svg"));
        const QByteArray source = file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray();
        sources.insert(name, source);
        return source;
    }

    // QIcon's own SVG engine cannot follow a mode: every state would need its own file.
    class TintedSvgEngine : public QIconEngine {
    public:
        TintedSvgEngine(const QString &name, const QColor &color) : name(name), color(color) {}

        void paint(QPainter *painter, const QRect &rect, QIcon::Mode mode, QIcon::State) override {
            QByteArray source = porosThemeIconSource(name);
            source.replace("currentColor", tint(mode).name(QColor::HexRgb).toLatin1());
            QSvgRenderer renderer(source);
            renderer.render(painter, rect);
        }

        QPixmap pixmap(const QSize &size, QIcon::Mode mode, QIcon::State state) override {
            return scaledPixmap(size, mode, state, 1.0);
        }

        QPixmap scaledPixmap(const QSize &size, QIcon::Mode mode, QIcon::State state, qreal scale) override {
            const QSize physical = size * scale;
            const QString key = QStringLiteral("poros-icon-%1-%2-%3x%4-%5")
                                    .arg(name, tint(mode).name(QColor::HexRgb))
                                    .arg(physical.width()).arg(physical.height()).arg(int(mode));
            QPixmap pixmap;
            if (!pixmapCache.contains(key)) {
                pixmap = QPixmap(physical);
                pixmap.fill(Qt::transparent);
                QPainter painter(&pixmap);
                painter.setRenderHint(QPainter::Antialiasing);
                paint(&painter, QRect(QPoint(0, 0), physical), mode, state);
                painter.end();
                pixmapCache.insert(key, pixmap);
            } else {
                pixmap = pixmapCache.value(key);
            }
            pixmap.setDevicePixelRatio(scale);
            return pixmap;
        }

        QSize actualSize(const QSize &size, QIcon::Mode, QIcon::State) override { return size; }
        QString key() const override { return QStringLiteral("PorosTintedSvg"); }
        QIconEngine *clone() const override { return new TintedSvgEngine(name, color); }
        QString iconName() override { return name; }

    private:
        QColor tint(QIcon::Mode mode) const {
            if (mode == QIcon::Disabled) return QColor(PorosTheme::TextDisabled);
            if (color.isValid()) return color;
            switch (mode) {
                case QIcon::Active: return QColor(PorosTheme::T1);
                case QIcon::Selected: return QColor(Qt::white);
                default: return QColor(PorosTheme::T2);
            }
        }

        QString name;
        QColor color;
        inline static QHash<QString, QPixmap> pixmapCache;
    };
} // namespace

namespace PorosTheme {
    QMap<QString, QString> StyleTokens() {
        const auto hex = [](QRgb rgb) { return QColor(rgb).name(QColor::HexRgb); };
        return {
            {"white", QStringLiteral("#FFFFFF")}, {"closered", QStringLiteral("#C42B3C")}, {"blue", hex(Blue)}, {"violet", hex(Violet)}, {"orchid", hex(Orchid)}, {"pink", hex(Pink)},
            {"s0", hex(S0)}, {"s1", hex(S1)}, {"s2", hex(S2)}, {"s3", hex(S3)}, {"s4", hex(S4)}, {"s5", hex(S5)},
            {"line", hex(Line)}, {"line2", hex(Line2)},
            {"t1", hex(T1)}, {"t2", hex(T2)}, {"t3", hex(T3)}, {"tdis", hex(TextDisabled)},
            {"act", hex(Act)}, {"gmid", hex(GradMid)}, {"gend", hex(GradEnd)},
            {"ok", hex(Ok)}, {"warn", hex(Warn)}, {"bad", hex(Bad)},
        };
    }

    QFont HeadingFont(int pixelSize) {
        QFont font;
        font.setFamilies({QStringLiteral("Unbounded"), QStringLiteral("Onest"), QStringLiteral("Segoe UI")});
        font.setPixelSize(pixelSize);
        return font;
    }

    QFont MonoFont(int pixelSize) {
        QFont font;
        font.setFamilies({QStringLiteral("JetBrains Mono"), QStringLiteral("Consolas")});
        font.setPixelSize(pixelSize);
        return font;
    }

    QLinearGradient PrimaryGradient(const QRectF &rect) {
        QLinearGradient gradient(rect.topLeft(), rect.topRight());
        gradient.setColorAt(0, QColor(Act));
        gradient.setColorAt(0.6, QColor(GradMid));
        gradient.setColorAt(1, QColor(GradEnd));
        return gradient;
    }

    QLinearGradient RoadGradient(const QRectF &rect) {
        QLinearGradient gradient(rect.topLeft(), rect.topRight());
        gradient.setColorAt(0, QColor(Blue));
        gradient.setColorAt(0.55, QColor(Violet));
        gradient.setColorAt(1, QColor(Orchid));
        return gradient;
    }

    QIcon Icon(const QString &name, const QColor &color) {
        return QIcon(new TintedSvgEngine(name, color));
    }
} // namespace PorosTheme
