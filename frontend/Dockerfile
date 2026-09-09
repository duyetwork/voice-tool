# syntax=docker/dockerfile:1

FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci || npm install

FROM node:22-alpine AS builder
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

# NEXT_PUBLIC_* được nướng vào bundle lúc build, nên phải truyền ở đây — đặt
# trong `environment` của container lúc chạy là KHÔNG có tác dụng.
#
# Bỏ trống = frontend gọi API bằng đường dẫn tương đối `/api/v1`, dùng khi cả
# hai đứng sau cùng 1 reverse proxy / 1 tên miền.
ARG NEXT_PUBLIC_API_URL=""
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000

RUN addgroup -g 1001 nodejs && adduser -D -u 1001 -G nodejs nextjs

# output: "standalone" đóng gói sẵn server + dependency cần thiết.
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
