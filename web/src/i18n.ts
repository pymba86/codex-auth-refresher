import { createContext, useContext } from "react";

export type Locale = "ru" | "en";

export type TranslationParams = Record<string, number | string>;

export const LOCALE_STORAGE_KEY = "codex-dashboard-locale";

const ru = {
  "app.brandSubtitle": "Панель состояния",
  "nav.overview": "Обзор",
  "nav.files": "Файлы",
  "nav.metrics": "Метрики",
  "nav.endpoints": "Эндпоинты",
  "sidebar.refreshPolicy": "Политика обновления",
  "sidebar.scanPolicy":
    "Интервал сканирования {interval}, параллелизм {parallel}.",
  "sidebar.serviceState": "Состояние сервиса",
  "sidebar.serviceUptime": "Аптайм {value}.",
  "app.title": "Состояние Codex-токенов",
  "app.subtitle":
    "Панель состояния: видно здоровье refresh-цикла, состояние auth-файлов и ближайшие обновления.",
  "top.autoRefresh": "Автообновление каждые {seconds} с",
  "top.updatedAt": "Обновлено {time}",
  "top.refresh": "Обновить",
  "top.languageSwitcher": "Выбор языка",
  "banner.loadingTitle": "Загрузка дашборда",
  "banner.loadingMessage":
    "Получаю текущее состояние refresh-цикла, список файлов и операционные метрики.",
  "banner.startingTitle": "Сервис запускается",
  "banner.startingMessage":
    "Планировщик ещё прогревается. Пока readiness не станет зелёным, данные могут быть неполными.",
  "banner.staleTitle": "Показываю последние успешные данные",
  "banner.staleMessage":
    "Последний запрос к API завершился ошибкой ({error}), но на экране остаётся предыдущий успешный снимок.",
  "banner.fetchFailedTitle": "Не удалось обновить дашборд",
  "banner.fetchFailedMessage":
    "Запрос к dashboard API завершился ошибкой: {error}.",
  "section.overview": "Обзор",
  "section.files": "Файлы",
  "section.metrics": "Метрики",
  "section.endpoints": "Эндпоинты",
  "summary.trackedFiles": "Отслеживаемые файлы",
  "summary.trackedFilesHelper": "Отключено: {count}",
  "summary.okFiles": "Исправные",
  "summary.okFilesHelper": "Цикл refresh работает стабильно",
  "summary.degradedFiles": "Проблемные",
  "summary.degradedFilesHelper": "Ошибок всего: {count}",
  "summary.reauthFiles": "Требуют входа",
  "summary.reauthFilesHelper": "Нужен новый вход",
  "summary.invalidJsonFiles": "Некорректный JSON",
  "summary.invalidJsonFilesHelper": "Парсер не смог прочитать файл",
  "summary.uptime": "Аптайм",
  "summary.uptimeHelper": "Запущен {time}",
  "summary.lastScan": "Последний скан",
  "summary.lastScanHelper": "Сканирований всего: {count}",
  "summary.refreshSuccess": "Успешные refresh",
  "summary.refreshSuccessHelper": "Попыток всего: {count}",
  "filters.all": "Все",
  "filters.ok": "OK",
  "filters.degraded": "Проблемные",
  "filters.reauthRequired": "Нужен вход",
  "filters.invalidJson": "Некорректные",
  "filters.searchPlaceholder": "Поиск по файлу или account_id",
  "filters.showDisabled": "Показывать отключённые файлы",
  "files.title": "Отслеживаемые auth-файлы",
  "files.meta": "Текущее состояние refresh для каждого Codex auth-документа.",
  "files.visibleCount": "Видимых записей: {count}",
  "files.empty": "По текущим фильтрам auth-файлы не найдены.",
  "files.column.file": "Файл",
  "files.column.account": "Аккаунт",
  "files.column.schema": "Схема",
  "files.column.state": "Состояние",
  "files.column.expiresAt": "Истекает",
  "files.column.nextRefreshAt": "Следующий refresh",
  "files.column.lastRefreshAt": "Последний refresh",
  "files.column.consecutiveFailures": "Ошибки подряд",
  "files.column.disabled": "Отключён",
  "files.column.error": "Ошибка",
  "metrics.refreshAttempts": "Попытки refresh",
  "metrics.refreshAttemptsHint": "Успешно: {success} · Ошибок: {failure}",
  "metrics.scans": "Сканирований",
  "metrics.scansHint": "Последний скан {time}",
  "metrics.readiness": "Готовность",
  "metrics.ready": "Готов",
  "metrics.starting": "Запуск",
  "metrics.startedAt": "Старт {time}",
  "metrics.statusApi": "Status API",
  "metrics.statusApiHint": "Доступность сырого JSON-статуса",
  "endpoint.dashboard": "Дашборд",
  "endpoint.uiJson": "JSON для UI",
  "endpoint.healthz": "Проверка жизни",
  "endpoint.readyz": "Готовность",
  "endpoint.metrics": "Метрики",
  "endpoint.rawStatus": "Сырой статус",
  "state.ok": "OK",
  "state.degraded": "Проблема",
  "state.reauth_required": "Нужен вход",
  "state.invalid_json": "Некорректный JSON",
  "common.yes": "Да",
  "common.no": "Нет",
  "common.enabled": "Включён",
  "common.disabled": "Выключен",
  "common.notAvailable": "—",
  "common.disabledValue": "выключено",
  "error.unknownDashboard": "неизвестная ошибка дашборда",
  "error.httpStatus": "HTTP {status}",
};

const en: typeof ru = {
  "app.brandSubtitle": "Status Dashboard",
  "nav.overview": "Overview",
  "nav.files": "Files",
  "nav.metrics": "Metrics",
  "nav.endpoints": "Endpoints",
  "sidebar.refreshPolicy": "Refresh Policy",
  "sidebar.scanPolicy": "Scan interval {interval}, concurrency {parallel}.",
  "sidebar.serviceState": "Service State",
  "sidebar.serviceUptime": "Uptime {value}.",
  "app.title": "Codex Token Status",
  "app.subtitle":
    "Dashboard for refresh cycle health, auth-file state, and upcoming refreshes.",
  "top.autoRefresh": "Auto-refresh every {seconds}s",
  "top.updatedAt": "Updated {time}",
  "top.refresh": "Refresh",
  "top.languageSwitcher": "Language switcher",
  "banner.loadingTitle": "Loading dashboard",
  "banner.loadingMessage":
    "Fetching refresh-cycle state, tracked files, and operational metrics.",
  "banner.startingTitle": "Service is starting",
  "banner.startingMessage":
    "The scheduler is still warming up. Data may be incomplete until readiness turns green.",
  "banner.staleTitle": "Showing the last successful snapshot",
  "banner.staleMessage":
    "The latest API request failed ({error}), but the previous successful snapshot is still on screen.",
  "banner.fetchFailedTitle": "Failed to refresh dashboard",
  "banner.fetchFailedMessage": "The dashboard API request failed: {error}.",
  "section.overview": "Overview",
  "section.files": "Files",
  "section.metrics": "Metrics",
  "section.endpoints": "Endpoints",
  "summary.trackedFiles": "Tracked Files",
  "summary.trackedFilesHelper": "Disabled: {count}",
  "summary.okFiles": "Healthy",
  "summary.okFilesHelper": "Refresh loop is stable",
  "summary.degradedFiles": "Degraded",
  "summary.degradedFilesHelper": "Total failures: {count}",
  "summary.reauthFiles": "Need Sign-In",
  "summary.reauthFilesHelper": "A new login is required",
  "summary.invalidJsonFiles": "Invalid JSON",
  "summary.invalidJsonFilesHelper": "The parser could not read the file",
  "summary.uptime": "Uptime",
  "summary.uptimeHelper": "Started {time}",
  "summary.lastScan": "Last Scan",
  "summary.lastScanHelper": "Total scans: {count}",
  "summary.refreshSuccess": "Successful Refreshes",
  "summary.refreshSuccessHelper": "Total attempts: {count}",
  "filters.all": "All",
  "filters.ok": "OK",
  "filters.degraded": "Degraded",
  "filters.reauthRequired": "Needs Sign-In",
  "filters.invalidJson": "Invalid",
  "filters.searchPlaceholder": "Search by file or account_id",
  "filters.showDisabled": "Show disabled files",
  "files.title": "Tracked Auth Files",
  "files.meta": "Current refresh status for each Codex auth document.",
  "files.visibleCount": "Visible entries: {count}",
  "files.empty": "No auth files match the current filters.",
  "files.column.file": "File",
  "files.column.account": "Account",
  "files.column.schema": "Schema",
  "files.column.state": "State",
  "files.column.expiresAt": "Expires",
  "files.column.nextRefreshAt": "Next Refresh",
  "files.column.lastRefreshAt": "Last Refresh",
  "files.column.consecutiveFailures": "Consecutive Failures",
  "files.column.disabled": "Disabled",
  "files.column.error": "Error",
  "metrics.refreshAttempts": "Refresh Attempts",
  "metrics.refreshAttemptsHint": "Success: {success} · Failures: {failure}",
  "metrics.scans": "Scans",
  "metrics.scansHint": "Last scan {time}",
  "metrics.readiness": "Readiness",
  "metrics.ready": "Ready",
  "metrics.starting": "Starting",
  "metrics.startedAt": "Started {time}",
  "metrics.statusApi": "Status API",
  "metrics.statusApiHint": "Availability of the raw JSON status",
  "endpoint.dashboard": "Dashboard",
  "endpoint.uiJson": "UI JSON",
  "endpoint.healthz": "Liveness",
  "endpoint.readyz": "Readiness",
  "endpoint.metrics": "Metrics",
  "endpoint.rawStatus": "Raw Status",
  "state.ok": "OK",
  "state.degraded": "Issue",
  "state.reauth_required": "Needs Sign-In",
  "state.invalid_json": "Invalid JSON",
  "common.yes": "Yes",
  "common.no": "No",
  "common.enabled": "Enabled",
  "common.disabled": "Disabled",
  "common.notAvailable": "—",
  "common.disabledValue": "disabled",
  "error.unknownDashboard": "unknown dashboard error",
  "error.httpStatus": "HTTP {status}",
};

const translations = { ru, en };

export type TranslationKey = keyof typeof ru;

export type I18nContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TranslationKey, params?: TranslationParams) => string;
};

export const I18nContext = createContext<I18nContextValue | null>(null);

function isLocale(value: string | null): value is Locale {
  return value === "ru" || value === "en";
}

function interpolate(template: string, params: TranslationParams = {}): string {
  return template.replace(/\{(\w+)\}/g, (_, key: string) =>
    String(params[key] ?? `{${key}}`),
  );
}

export function detectInitialLocale(): Locale {
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (isLocale(stored)) {
    return stored;
  }

  const browserLocale = window.navigator.language.toLowerCase();
  return browserLocale.startsWith("ru") ? "ru" : "en";
}

export function translate(
  locale: Locale,
  key: TranslationKey,
  params?: TranslationParams,
): string {
  return interpolate(translations[locale][key], params);
}

export function useI18n() {
  const value = useContext(I18nContext);
  if (!value) {
    throw new Error("useI18n must be used inside I18nProvider");
  }
  return value;
}
