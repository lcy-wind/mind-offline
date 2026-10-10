FROM node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts --no-audit --no-fund --registry=https://registry.npmjs.org && npm cache clean --force
COPY server.cjs playback.cjs kugou.cjs ./
USER node
ENV NODE_ENV=production NODE_OPTIONS=--max-old-space-size=128
CMD ["node", "server.cjs"]
