ARG STEAMCMD_IMAGE=cm2network/steamcmd:root
FROM ${STEAMCMD_IMAGE}
USER root
RUN apt-get update && apt-get install -y --no-install-recommends python3 util-linux lib32gcc-s1 \
    && rm -rf /var/lib/apt/lists/* \
    && (id -u steam >/dev/null 2>&1 || useradd -m -u 10001 steam)
COPY supervisor.py /usr/local/lib/l4d2-supervisor.py
COPY l4d2-supervisor /usr/local/bin/l4d2-supervisor
RUN chmod 0755 /usr/local/bin/l4d2-supervisor /usr/local/lib/l4d2-supervisor.py && mkdir -p /opt/l4d2/game /opt/l4d2/private /opt/l4d2/shared-vpk && chown -R steam:steam /opt/l4d2
USER steam
WORKDIR /opt/l4d2/game
ENTRYPOINT ["python3","/usr/local/lib/l4d2-supervisor.py","run"]
