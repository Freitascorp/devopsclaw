---
name: weather
description: Get current weather and forecasts (no API key required).
homepage: https://wttr.in/:help
metadata: {"nanobot":{"emoji":"🌤️","requires":{"bins":["curl"]}}}
---

# Weather

Two free services, no API keys needed.

## wttr.in (primary)

Quick one-liner:
```bash
curl -s "wttr.in/London?format=3"
# Output: London: ⛅️ +8°C
```

Compact format:
```bash
curl -s "wttr.in/London?format=%l:+%c+%t+%h+%w"
# Output: London: ⛅️ +8°C 71% ↙5km/h
```

Full forecast:
```bash
curl -s "wttr.in/London?T"
```

Format codes: `%c` condition · `%t` temp · `%h` humidity · `%w` wind · `%l` location · `%m` moon

Tips:
- URL-encode spaces: `wttr.in/New+York`
- Airport codes: `wttr.in/JFK`
- Units: `?m` (metric) `?u` (USCS)
- Today only: `?1` · Current only: `?0`
- PNG: `curl -s "wttr.in/Berlin.png" -o /tmp/weather.png`

## IPMA — Portugal weather (JSON API, no key)

For Portuguese cities, use the IPMA open-data API directly (the website is JS-rendered and won't work with web_fetch).

City ID lookup — common cities:
- **Aveiro**: 1010500
- **Lisboa**: 1110600
- **Porto**: 1131200
- **Faro**: 0806000
- **Coimbra**: 0603100
- **Braga**: 0303200

Full city list:
```bash
curl -s "https://api.ipma.pt/open-data/distrits-islands.json" | jq '.data[] | {local, globalIdLocal}'
```

Current day forecast for a city:
```bash
# Aveiro (ID 1010500)
curl -s "https://api.ipma.pt/open-data/forecast/meteorology/cities/daily/1010500.json" | jq '.data[0]'
```

Response fields: `tMin`, `tMax` (°C), `precipitaProb` (%), `predWindDir` (wind direction), `classWindSpeed` (1-4), `idWeatherType` (weather code).

Weather type codes:
```bash
curl -s "https://api.ipma.pt/open-data/weather-type-classe.json" | jq '.data'
```
Common codes: 1=Céu limpo, 2=Pouco nublado, 3=Parcialmente nublado, 4=Muito nublado, 6=Chuva, 9=Chuva forte, 18=Neve

5-day forecast:
```bash
curl -s "https://api.ipma.pt/open-data/forecast/meteorology/cities/daily/hp-daily-forecast-day0.json" | jq '.data[] | select(.globalIdLocal == 1010500)'
```

## Open-Meteo (worldwide JSON, no key)

Free, no key, good for programmatic use:
```bash
curl -s "https://api.open-meteo.com/v1/forecast?latitude=51.5&longitude=-0.12&current_weather=true"
```

Find coordinates for a city, then query. Returns JSON with temp, windspeed, weathercode.

Docs: https://open-meteo.com/en/docs
