# Kokoro TTS

Sintetiza voz mediante la API HTTP de [Kokoro](https://github.com/remsky/Kokoro-FastAPI) (endpoint compatible con OpenAI `/v1/audio/speech`) y la reproduce localmente a través de `mpv`, `mplayer`, `ffplay` o `vlc`.

Los archivos de audio se almacenan en caché por voz + hash del mensaje, por lo que frases idénticas nunca se vuelven a solicitar al servidor.

---

## Requisitos previos

Uno de los siguientes reproductores de audio debe estar instalado y disponible en `$PATH`:

| Reproductor | Instalar (Arch)      | Instalar (Ubuntu/Debian)  |
|-------------|----------------------|---------------------------|
| `mpv`       | `pacman -S mpv`      | `apt install mpv`         |
| `mplayer`   | `pacman -S mplayer`  | `apt install mplayer`     |
| `ffplay`    | `pacman -S ffmpeg`   | `apt install ffmpeg`      |
| `vlc`       | `pacman -S vlc`      | `apt install vlc`         |

En **Windows**, la reproducción usa la API MCI integrada del sistema — no se necesita software adicional.

---

## Opción A — Servicio local (auto-alojado)

Ejecuta el servidor Kokoro FastAPI en tu propia máquina. No se requiere clave de API.

### 1. Iniciar el servidor

La forma más sencilla es mediante Docker:

```bash
# Solo CPU
docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest

# GPU NVIDIA (mucho más rápido)
docker run --gpus all -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-gpu:latest
```

El servidor escucha en `http://localhost:8880` de forma predeterminada.

### Solución alternativa — Docker no detecta la GPU

Si el contenedor GPU falla con un error como `unknown or invalid runtime name: nvidia`
o `could not select device driver "nvidia"`, significa que el NVIDIA Container Toolkit
no está instalado o no está configurado.

**1. Instalar el toolkit**

```bash
# Arch Linux
yay -S nvidia-container-toolkit

# Ubuntu / Debian
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey \
  | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
  | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
  | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

sudo apt update && sudo apt install -y nvidia-container-toolkit
```

**2. Configurar el runtime de Docker**

```bash
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

**3. Verificar**

```bash
docker run --rm --gpus all nvidia/cuda:12.0-base-ubuntu22.04 nvidia-smi
```

Deberías ver tu GPU listada. Si `nvidia-smi` reporta un error, asegúrate de que
el driver NVIDIA del host esté instalado (`nvidia-smi` fuera de Docker debe
funcionar primero).

**4. Volver a ejecutar el contenedor GPU**

```bash
docker run --gpus all -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-gpu:latest
```

Si el acceso a la GPU no está disponible, usa la imagen CPU como alternativa —
la calidad es idéntica, solo difiere la velocidad.

### 2. `config.toml`

```toml
[tts]
engine   = "kokoro"
base_url = "http://localhost:8880"   # predeterminado; omitir si no cambia
voice    = "af_sky"                  # ver referencia de voces más abajo
model    = "kokoro"                  # único modelo soportado actualmente
volume   = 100                       # 0–200, 100 = nivel original
```

No se necesita `api_key_env` para un servidor local — el campo se ignora cuando
el servidor no requiere autenticación.

---

## Opción B — Servicio en la nube

### 1. Obtener una clave de API

Regístrate en el proveedor de Kokoro en la nube de tu elección y copia la clave de API.

### 2. Exportar la clave

```bash
export KOKORO_API_KEY="sk-..."
```

Agrega esa línea a `~/.bashrc` / `~/.zshrc` para que sea permanente.

### 3. `config.toml`

```toml
[tts]
engine      = "kokoro"
base_url    = "https://api.ejemplo-kokoro-cloud.com"   # endpoint en la nube
api_key_env = "KOKORO_API_KEY"                          # variable de entorno con la clave
voice       = "af_sky"
model       = "kokoro"
volume      = 100
```

Establece `base_url` con la URL que proporcione tu proveedor. El cliente agrega
`/v1/audio/speech` al valor de `base_url` que configures.

---

## Referencia completa de `config.toml`

```toml
[tts]
# Requerido
engine = "kokoro"

# URL de la API HTTP de Kokoro. Predeterminado: "http://localhost:8880".
base_url = "http://localhost:8880"

# Identificador de voz (ver referencia de voces más abajo). Requerido.
voice = "af_sky"

# Nombre del modelo enviado en cada solicitud. Valor predeterminado / único soportado: "kokoro".
model = "kokoro"

# Nombre de la variable de entorno que contiene la clave de API.
# Omitir o dejar en blanco para servidores locales sin autenticación.
# api_key_env = "KOKORO_API_KEY"

# Volumen de reproducción como porcentaje (0–200). 100 = nivel original, 150 = +50%.
volume = 100
```

---

## Referencia de voces

Las voces siguen el patrón `<acento><género>_<nombre>`. El prefijo codifica el
acento y el género:

| Prefijo | Acento           | Género   |
|---------|------------------|----------|
| `af_`   | Americano        | Femenino |
| `am_`   | Americano        | Masculino|
| `bf_`   | Británico        | Femenino |
| `bm_`   | Británico        | Masculino|

Ejemplos:

| Voz          | Descripción              |
|--------------|--------------------------|
| `af_sky`     | Femenina americana       |
| `af_bella`   | Femenina americana       |
| `am_adam`    | Masculina americana      |
| `bf_emma`    | Femenina británica       |
| `bm_daniel`  | Masculina británica      |
| `bm_lewis`   | Masculina británica      |

Ejecuta el servidor local y llama a `GET /v1/voices` para listar todas las voces
disponibles.

---

## Caché de audio

Los archivos MP3 sintetizados se almacenan en el directorio configurado mediante
el constructor de `Speaker` (parámetro `dir`), que en la práctica es definido por
`cmd/main.go`. Los archivos se nombran `kokoro-<sha256>.mp3`, donde el hash
incluye la voz y el texto del mensaje. Elimina el directorio para limpiar la
caché y forzar una nueva síntesis.
