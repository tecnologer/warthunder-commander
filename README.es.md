# War Thunder Commander

Asistente de combate en tiempo real para War Thunder. Lee la API HTTP local del
juego (`localhost:8111`) y emite alertas de voz para detección de enemigos,
amenazas de flanqueo y zonas de captura en disputa. Cada 30 segundos también
llama a un LLM (Groq o Anthropic) para entregar un informe táctico de una línea
por voz.

## Instalación

### Recomendado: asistente de configuración

Descarga `warthunder-setup` desde la página de [Releases](../../releases) y ejecútalo:

> `warthunder-commander` es un archivo de versión separado — el asistente lo descarga e instala por ti.

Ejecuta el asistente y sigue las instrucciones en pantalla:

```bash
./warthunder-setup
```

El asistente realizará lo siguiente:

1. Preguntará dónde instalar el binario (predeterminado: `~/.local/bin/wtcommander`).
2. Te guiará por cada opción de configuración (idioma, motor IA/clave, motor TTS, colores, etc.).
3. Descargará el binario `warthunder-commander` correcto para tu sistema operativo y arquitectura desde GitHub Releases.
4. Escribirá `warthunder-commander.toml` junto al binario.

### Compilación manual (desarrolladores)

```bash
git clone https://github.com/tecnologer/warthunder
cd warthunder
go build -o warthunder-commander ./cmd/main.go
```

## Uso

```bash
# Ejecutar con config.toml predeterminado en el directorio actual
./warthunder-commander

# Ejecutar en modo depuración (escribe respuestas crudas de la API de WT + logs JSONL por partida)
./warthunder-commander --debug

# Información de versión
./warthunder-commander --version
```

El asistente comienza a sondear la API del juego cada 500 ms y espera en
silencio hasta que comience una partida. Una vez que el objeto del jugador
aparece en 6 fotogramas consecutivos (≈3 s), anuncia "Partida iniciada" y
comienza a emitir alertas.

Usa `--debug` para escribir las respuestas crudas de la API en un archivo JSONL
con marca de tiempo. Los logs de partida (alertas y prompts/respuestas del
comandante) también se escriben cuando `log_dir` está configurado en
`config.toml` y `--debug` está activo.

## Configuración

Copia `config.toml` en el mismo directorio que el binario y edítalo. Todos los
campos son opcionales; los valores predeterminados se muestran a continuación.

```toml
# Idioma de la interfaz para alertas, voz del comandante y respuestas del LLM.
# Valores válidos: "es" (español, predeterminado), "en" (inglés).
language = "en"

[ai]
# Backend de IA: "groq" (predeterminado) o "anthropic".
engine = "groq"

# Nombre del modelo LLM; usa el predeterminado del motor si se omite.
# Predeterminado Groq: "llama-3.3-70b-versatile"
# Predeterminado Anthropic: "claude-sonnet-4-6"
# model = "llama-3.3-70b-versatile"

# Modo de personalidad del comandante.
# "warning" (predeterminado) — alertas situacionales, sin verbos de acción
# "orders"                   — comandos tácticos directos ("Reposicionarse en B4")
# "suggestions"              — recomendaciones suaves ("Considera reposicionarte")
mode = "warning"

# Distintivo del asistente — cómo el LLM se dirige a ti (máx. 3 palabras / 24 chars).
callsign = "Bronco"

# Número de alertas recientes incluidas como contexto para que el LLM varíe la redacción.
# Predeterminado: 3. Establece en 0 para deshabilitar.
alert_history_max = 3

# Variable de entorno que contiene la clave de API de Groq.
groq_env = "GROQ_API_KEY"

# Variable de entorno que contiene la clave de API de Anthropic.
anthropic_env = "ANTHROPIC_API_KEY"

[notifications]
# Nivel mínimo de prioridad para entregar. Acepta 1–4 o la cadena "commander".
#   1 = Info      — todas las alertas + comandante (predeterminado)
#   2 = Warning   — Advertencia, Crítico y Comandante
#   3 = Critical  — solo Crítico y Comandante
#   4 / "commander" — solo informes del Comandante (alertas regulares silenciadas)
min_priority = 1

[colors]
# Tolerancia RGB por canal para identificación de equipos.
tolerance = 30

[colors.player]
r = 250
g = 200
b = 30

[colors.ally]
r = 23
g = 77
b = 255

[colors.enemy]
r = 250
g = 12
b = 0

[colors.squad]
r = 103
g = 215
b = 86

[tts]
# Motor: "google-tts" (predeterminado, sin clave), "kokoro" o "camb".
engine = "kokoro"

# Volumen de reproducción como porcentaje (0–200, predeterminado 100).
volume = 150

# Multiplicador de velocidad de reproducción (0.25–4.0, predeterminado 1.0).
speed = 1.0

# --- Configuración de Kokoro ---
# Ver internal/tts/kokoro/README.es.md para detalles completos de configuración de Kokoro.

# --- Configuración de CAMB.AI ---
# Ver internal/tts/camb/README.es.md para detalles completos de configuración de CAMB.AI.
```

### Motores TTS

| Motor | Clave requerida | Notas |
|-------|----------------|-------|
| `google-tts` | No | Usa Google Translate; requiere acceso a internet |
| `kokoro` | Opcional | Servidor local `/v1/audio/speech` compatible con OpenAI; ver [`internal/tts/kokoro/README.es.md`](internal/tts/kokoro/README.es.md) para configuración y referencia de voces |
| `camb` | Sí (`CAMB_API_KEY`) | Voces en la nube; ver [`internal/tts/camb/README.es.md`](internal/tts/camb/README.es.md) para configuración y descubrimiento de voces |

## Arquitectura

```
main.go  →  wt.Client.MapObjects()  →  analyzer.Analyze()  →  tts.Speaker.Speak()
                                    →  collector.Add()
                                    →  commander.Advise()   ↗
```

## Características

- **Detección de flanqueo** — avisa cuando un enemigo se acerca dentro del 15%
  del ancho del mapa en un ángulo mayor a 90° respecto a tu dirección (prioridad
  Crítica).
- **Enemigo detectado** — anuncia nuevos enemigos por tipo de unidad, cantidad y
  dirección de movimiento. Agrupa detecciones en una ventana de 1 segundo en una
  sola alerta.
- **Presión en zona** — se activa cuando una zona de captura está en disputa y
  hay un enemigo dentro del 8% del ancho del mapa.
- **Comandante IA** — cada 30 segundos un LLM de Groq o Anthropic lee el
  resumen del campo de batalla de los últimos 30 segundos y emite un informe
  táctico breve (≤15 palabras). Tres modos de comandante: `warning` (alertas
  situacionales), `orders` (comandos tácticos directos) o `suggestions`
  (recomendaciones suaves).
- **Conciencia del modo de juego** — Arcade muestra todos los enemigos;
  Realista solo muestra enemigos activamente detectados por un aliado cercano;
  Simulador suprime todas las alertas de enemigos.
- **Seguimiento de enemigos** — rastrea enemigos y miembros del escuadrón entre
  fotogramas por proximidad, con retención de 30 segundos (60 segundos para
  contactos cercanos). Evita re-alertar sobre la misma amenaza dentro de una
  ventana de silencio de 30 segundos.
- **Filtro de notificaciones** — `min_priority` configurable silencia alertas de
  baja prioridad (ej. `min_priority = 3` entrega solo alertas Críticas y del
  Comandante).
- **Bilingüe** — soporte completo en inglés y español para todas las alertas,
  voz del comandante y respuestas del LLM.
- **TTS intercambiable** — Google Translate (gratuito, sin clave), Kokoro (API
  local compatible con OpenAI) o CAMB.AI.
- **Caché de audio** — los MP3 sintetizados se almacenan en `/tmp/wt-tts/` y se
  reutilizan para cadenas idénticas.
- **Silencioso en reposo** — cuando el juego no está en ejecución, el ciclo
  opera en silencio sin generar ruido en los logs.

## Requisitos

- Go 1.21+
- `mplayer` o `mpv` instalado y en `$PATH`
- War Thunder en ejecución (la API local solo está activa dentro del juego)
- Para comandante Groq: variable de entorno `GROQ_API_KEY`
- Para comandante Anthropic: variable de entorno `ANTHROPIC_API_KEY`
- Para TTS CAMB.AI: variable de entorno `CAMB_API_KEY`
- Para TTS Kokoro: un servidor Kokoro local en `http://localhost:8880` (o
  cualquier endpoint `/v1/audio/speech` compatible con OpenAI)

### Reglas de detección (orden de prioridad)

1. **Crítico** — enemigo dentro del 15% del ancho del mapa a >90° de la dirección del jugador → alerta de flanqueo con lado.
2. **Advertencia** — nuevo enemigo confirmado (visto en dos fotogramas consecutivos) → alerta de detección agrupada.
3. **Advertencia** — zona de captura en disputa con enemigo dentro del 8% del ancho del mapa.

Como máximo, una alerta se activa por ventana de enfriamiento de 4 segundos; gana la prioridad más alta.

### Identificación de equipos

Los equipos se identifican comparando el arreglo RGB `color[]` de `map_obj.json`
con colores de referencia configurables con una tolerancia por canal (predeterminado ±30).
No existe un campo de ID de equipo en la API.

## Desarrollo

```bash
# Ejecutar pruebas
go test ./...

# Ejecutar pruebas con salida detallada
go test -v ./...

# Linting
golangci-lint run

# Compilar con versión
go build -ldflags "-X main.version=v1.0.0" -o warthunder-commander ./cmd/main.go
```

## API local de War Thunder

`localhost:8111` solo está activa mientras el juego está en ejecución. Endpoints principales:

| Endpoint | Uso |
|----------|-----|
| `/map_obj.json` | Posiciones del jugador, aliados, enemigos y zonas de captura por fotograma |
| `/map_info.json` | Nombre del mapa y dimensiones |

Las coordenadas en `map_obj.json` están normalizadas a `[0.0, 1.0]`. El objeto
del jugador incluye vectores de dirección `dx`/`dy`; los demás objetos no.
