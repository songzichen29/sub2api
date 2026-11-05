FROM node:24-bookworm-slim
WORKDIR /opt/qqbot
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund && npm cache clean --force
COPY src ./src
COPY profile.yml ./profile.yml
COPY astrbot_plugin ./astrbot_plugin
COPY test ./test
RUN node --test test/*.test.mjs
ENV NODE_ENV=production DSH_TELEMETRY_DISABLED=1 DSH_HOME=/data/dsh
USER node
EXPOSE 8787 6200
CMD ["node", "src/server.mjs"]
