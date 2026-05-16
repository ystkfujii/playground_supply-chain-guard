FROM node:24-alpine

ENV NODE_ENV=production \
    PORT=8080

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app -u 10001

COPY package.json ./
RUN npm install --omit=dev && npm cache clean --force

COPY server.js ./
RUN chown -R app:app /app

USER 10001
EXPOSE 8080
CMD ["node", "server.js"]
