#!/bin/bash
# e2e_test.sh: 端到端测试
BASE=http://127.0.0.1:8080

# 1. 登录
echo "=== 1. 登录 ==="
TOKEN=$(curl -s -X POST $BASE/api/v1/auth/login -H 'Content-Type: application/json' -d @/tmp/login.json | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
echo "TOKEN obtained"

# 2. 上传文档
echo "=== 2. 上传文档 ==="
echo "This is a test print document for Cloud Print System." > /tmp/test_doc.txt
UPLOAD_RESP=$(curl -s -X POST $BASE/api/v1/documents/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/test_doc.txt")
echo "$UPLOAD_RESP"
DOC_ID=$(echo "$UPLOAD_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['doc_id'])")
echo "DOC_ID=$DOC_ID"

# 3. 查询设备
echo "=== 3. 查询设备 ==="
curl -s $BASE/api/v1/devices -H "Authorization: Bearer $TOKEN"
echo ""

# 4. 创建打印任务
echo "=== 4. 创建打印任务 ==="
DEVICE_ID="3aec85cd-8819-4cc0-8dee-343a166cca33"
TASK_RESP=$(curl -s -X POST $BASE/api/v1/tasks/ \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"device_id\":\"$DEVICE_ID\",\"doc_id\":\"$DOC_ID\"}")
echo "$TASK_RESP"
TASK_ID=$(echo "$TASK_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['task_id'])" 2>/dev/null || echo "")
echo "TASK_ID=$TASK_ID"

# 5. 查询任务状态
echo "=== 5. 查询任务状态 ==="
sleep 3
if [ -n "$TASK_ID" ]; then
  curl -s $BASE/api/v1/tasks/$TASK_ID/ -H "Authorization: Bearer $TOKEN"
  echo ""
fi

# 6. 列出所有任务
echo "=== 6. 任务列表 ==="
curl -s "$BASE/api/v1/tasks/" -H "Authorization: Bearer $TOKEN"
echo ""