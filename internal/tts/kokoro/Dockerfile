FROM ghcr.io/remsky/kokoro-fastapi-gpu:latest
RUN python -m ensurepip --default-pip
RUN python -m pip uninstall -y torch
RUN python -m pip install --upgrade torch --index-url https://download.pytorch.org/whl/cu128
