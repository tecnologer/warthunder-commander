# Motor TTS CAMB.AI

Este paquete integra las voces en la nube de [CAMB.AI](https://camb.ai) en el comandante de War Thunder.

## Requisitos

- Una cuenta en CAMB.AI y su API key
- `mplayer` o `mpv` instalado para reproducir audio

## Configuración

Edita `config.toml` y ajusta la sección `[tts]`:

```toml
[tts]
engine = "camb"

# Nombre de la variable de entorno que contiene tu API key de CAMB.AI.
# Valor por defecto: "CAMB_API_KEY"
api_key_env = "CAMB_API_KEY"

# Voz: ID numérico de CAMB.AI o nombre exacto de la voz.
# Ejecuta el comando list-voices (ver más abajo) para ver las opciones disponibles.
voice = "165304"

# Código de idioma BCP-47 para la síntesis. Por defecto: "es-mx".
# Ejemplos: "en-us", "es-mx", "es-es", "pt-br"
language = "es-mx"

# Volumen de reproducción como porcentaje (0–200, por defecto 100).
volume = 100

# Multiplicador de velocidad de reproducción (0.25–4.0, por defecto 1.0).
speed = 1.0
```

### API key

Exporta tu clave de CAMB.AI antes de ejecutar la aplicación:

```bash
export CAMB_API_KEY=tu_clave_aqui
go run cmd/main.go
```

O agrégala a tu perfil de shell (`.bashrc`, `.zshrc`, etc.) para hacerla permanente.

## Descubrir voces disponibles

Lista todas las voces disponibles para un idioma:

```bash
CAMB_API_KEY=tu_clave_aqui ./warthunder-commander camb list voices --lang es-mx
```

Omite `--lang` para listar todas las voces sin filtrar por idioma.

El comando muestra el ID numérico, el nombre y el género de cada voz; copia el ID o el nombre directamente en `config.toml`.

## Cómo funciona

1. Al iniciar, el paquete resuelve el valor de `language` a un ID numérico de idioma en CAMB.AI consultando `/source-languages`.
2. El valor de `voice` se resuelve a un ID numérico de voz consultando `/list-voices` (acepta tanto un entero como un nombre exacto de voz).
3. Cada llamada a `Speak` envía un trabajo TTS a `/apis/tts`, hace polling a `/apis/tts/{task_id}` hasta que el trabajo finalice (máximo 120 s), y descarga el MP3 resultante desde `/apis/tts-result/{run_id}`.
4. El MP3 se almacena en caché localmente (usando un hash SHA-256 de voz + idioma + texto), de modo que las frases repetidas nunca vuelven a consumir red.
5. El archivo en caché se reproduce con `mplayer`/`mpv`.

## Ubicación de la caché

Los archivos de audio se guardan en el directorio configurado por `log_dir` en `config.toml` (o en `/tmp/wt-tts/` por defecto). Cada archivo se nombra `camb-<sha256>.mp3`.
