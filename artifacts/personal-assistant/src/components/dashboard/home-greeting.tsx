import { useEffect, useState } from 'react';
import { CloudSun, LoaderCircle, MapPin } from 'lucide-react';
import { useAppAuth } from '@/contexts/auth-context';
import { useLocale } from '@/contexts/locale-context';

type CurrentWeather = {
  temperature: number;
  code: number;
};

function weatherLabel(code: number, locale: string) {
  const arabic = locale === 'ar';
  if (code === 0) return arabic ? 'سماء صافية' : 'Clear';
  if (code <= 3) return arabic ? 'غائم جزئيًا' : 'Partly cloudy';
  if (code <= 48) return arabic ? 'ضباب' : 'Fog';
  if (code <= 57) return arabic ? 'رذاذ' : 'Drizzle';
  if (code <= 67 || (code >= 80 && code <= 82)) return arabic ? 'ممطر' : 'Rain';
  if (code <= 77 || (code >= 85 && code <= 86)) return arabic ? 'ثلوج' : 'Snow';
  return arabic ? 'عواصف رعدية' : 'Thunderstorm';
}

function greetingForHour(hour: number, locale: string) {
  if (locale === 'ar') {
    if (hour < 12) return 'صباح الخير';
    if (hour < 17) return 'مساء الخير';
    return 'مساء الخير';
  }
  if (hour < 12) return 'Good morning';
  if (hour < 17) return 'Good afternoon';
  return 'Good evening';
}

export function HomeGreeting() {
  const { user } = useAppAuth();
  const { locale, formatDate, formatNumber } = useLocale();
  const [now, setNow] = useState(() => new Date());
  const [weather, setWeather] = useState<CurrentWeather | null>(null);
  const [loadingWeather, setLoadingWeather] = useState(false);
  const [weatherError, setWeatherError] = useState('');

  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 30_000);
    return () => window.clearInterval(timer);
  }, []);

  const loadWeather = () => {
    if (!navigator.geolocation) {
      setWeatherError(locale === 'ar' ? 'الموقع غير متاح في هذا المتصفح.' : 'Location is unavailable in this browser.');
      return;
    }

    setLoadingWeather(true);
    setWeatherError('');
    setWeather(null);
    navigator.geolocation.getCurrentPosition(async ({ coords }) => {
      if (!Number.isFinite(coords.latitude) || Math.abs(coords.latitude) > 90 ||
          !Number.isFinite(coords.longitude) || Math.abs(coords.longitude) > 180) {
        setWeatherError(locale === 'ar' ? 'تعذر تحديد موقع صالح.' : 'Could not get a valid location.');
        setLoadingWeather(false);
        return;
      }

      const controller = new AbortController();
      const timeout = window.setTimeout(() => controller.abort(), 8_000);
      try {
        const url = new URL('https://api.open-meteo.com/v1/forecast');
        url.search = new URLSearchParams({
          latitude: String(coords.latitude),
          longitude: String(coords.longitude),
          current: 'temperature_2m,weather_code',
          timezone: 'auto',
        }).toString();
        const response = await fetch(url, { signal: controller.signal });
        if (!response.ok) throw new Error('weather');
        const payload: unknown = await response.json();
        if (!payload || typeof payload !== 'object' || !('current' in payload)) throw new Error('weather');
        const current = payload.current;
        if (!current || typeof current !== 'object' ||
            !('temperature_2m' in current) || !('weather_code' in current)) {
          throw new Error('weather');
        }
        const temperature = current.temperature_2m;
        const code = current.weather_code;
        if (typeof temperature !== 'number' || !Number.isFinite(temperature) ||
            temperature < -90 || temperature > 70 ||
            typeof code !== 'number' || !Number.isInteger(code) || code < 0 || code > 99) {
          throw new Error('weather');
        }
        setWeather({ temperature, code });
      } catch {
        setWeatherError(locale === 'ar' ? 'تعذر تحميل الطقس الآن.' : 'Weather is unavailable right now.');
      } finally {
        window.clearTimeout(timeout);
        setLoadingWeather(false);
      }
    }, () => {
      setWeatherError(locale === 'ar' ? 'اسمح بالوصول إلى موقعك لعرض الطقس.' : 'Allow location access to show local weather.');
      setLoadingWeather(false);
    }, { enableHighAccuracy: false, maximumAge: 15 * 60_000, timeout: 10_000 });
  };

  const name = user?.firstName?.trim();
  const greeting = greetingForHour(now.getHours(), locale);
  const date = formatDate(now, { weekday: 'long', month: 'long', day: 'numeric' });
  const time = formatDate(now, { hour: 'numeric', minute: '2-digit' });
  const privacyNote = locale === 'ar'
    ? 'يرسل الموقع إلى Open-Meteo لهذا الطلب فقط، ولا يحفظه Askolo.'
    : 'Your location is sent to Open-Meteo for this request; Askolo does not save it.';

  return (
    <header className="flex flex-col gap-5 border-b border-border pb-6 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <p className="text-sm text-muted-foreground">
          <time dateTime={now.toISOString()}>{date} · {time}</time>
        </p>
        <h1 className="mt-2 text-3xl font-display font-semibold tracking-tight md:text-4xl">
          {greeting}{name ? <>, <bdi dir="auto">{name}</bdi></> : ''}
        </h1>
      </div>

      <div className="sm:max-w-64">
        <button
          type="button"
          onClick={loadWeather}
          disabled={loadingWeather}
          aria-describedby="home-weather-privacy"
          className="inline-flex min-h-11 max-w-full items-center gap-2 rounded-xl border border-border bg-card/70 px-3 py-2 text-start text-sm shadow-sm transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-wait disabled:opacity-70"
          data-testid="button-home-weather"
        >
          {loadingWeather
            ? <LoaderCircle className="size-4 shrink-0 animate-spin" aria-hidden="true" />
            : weather
              ? <CloudSun className="size-4 shrink-0 text-primary" aria-hidden="true" />
              : <MapPin className="size-4 shrink-0 text-primary" aria-hidden="true" />}
          <span className="min-w-0">
            <span className="block truncate font-medium" role="status" aria-live="polite">
              {weather
                ? `${formatNumber(Math.round(weather.temperature))}° · ${weatherLabel(weather.code, locale)}`
                : loadingWeather
                  ? (locale === 'ar' ? 'جارٍ تحميل الطقس…' : 'Checking weather…')
                  : weatherError || (locale === 'ar' ? 'عرض الطقس المحلي' : 'Show local weather')}
            </span>
          </span>
        </button>
        <p id="home-weather-privacy" className="mt-1 text-xs text-muted-foreground">
          {privacyNote}
        </p>
      </div>
    </header>
  );
}
