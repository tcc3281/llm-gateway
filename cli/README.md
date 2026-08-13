# LLM Gateway CLI (Python)

Đây là công cụ CLI quản trị cổng kết nối LLM Gateway, được viết bằng Python và quản lý bởi `uv`.

## Cài đặt nhanh với `uv`

Nếu bạn chưa cài `uv`, hãy cài đặt nó trước:
```bash
# macOS/Linux
curl -LsSf https://astral.sh/uv/install.sh | sh
```

Cài đặt CLI dưới dạng công cụ toàn cục (global tool):
```bash
uv tool install --editable ./cli
```
Sau đó, bạn có thể chạy lệnh `llmgw` từ bất cứ đâu trên terminal.

## Chạy không cần cài đặt (Development)

Trong thư mục gốc của dự án, bạn có thể sử dụng `uv run` để thực thi CLI ngay lập tức:
```bash
uv run --project cli llmgw --help
```

## Biến môi trường cấu hình

CLI kết nối đến cổng Admin của LLM Gateway thông qua hai biến môi trường chính:
* `GATEWAY_URL`: URL của Admin Server (mặc định: `http://localhost:8081`).
* `ADMIN_API_KEY`: API Key quản trị của Gateway (mặc định: `sk-admin-secret-key-change-me`).

Ví dụ:
```bash
export GATEWAY_URL="http://localhost:8081"
export ADMIN_API_KEY="sk-admin-secret-key-12345"
llmgw health
```
