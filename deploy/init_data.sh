#!/bin/bash
# init_data.sh: 初始化工厂、Agent、设备
BASE=http://127.0.0.1:8080

# 登录获取 token
TOKEN=$(curl -s -X POST $BASE/api/v1/auth/login -H 'Content-Type: application/json' -d @/tmp/login.json | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
echo "=== TOKEN 获取成功 ==="

# 1. 创建工厂
echo "=== 创建工厂 ==="
FACTORY_RESP=$(curl -s -X POST $BASE/api/v1/admin/factories/ \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"宝山工厂","code":"BS","location":"上海宝山"}')
echo "$FACTORY_RESP"
FACTORY_ID=$(echo "$FACTORY_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['factory_id'])")
echo "FACTORY_ID=$FACTORY_ID"

# 2. 注册Agent
echo "=== 注册Agent ==="
AGENT_RESP=$(curl -s -X POST $BASE/api/v1/admin/agents/ \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"宝山Agent\",\"factory_id\":\"$FACTORY_ID\",\"version\":\"0.1.0\"}")
echo "$AGENT_RESP"
AGENT_ID=$(echo "$AGENT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['agent_id'])")
AGENT_TOKEN=$(echo "$AGENT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
echo "AGENT_ID=$AGENT_ID"
echo "AGENT_TOKEN=$AGENT_TOKEN"

# 3. 添加设备 (EPSON LQ-630KII via 192.168.2.81:9100)
echo "=== 添加设备 ==="
curl -s -w '\nHTTP:%{http_code}\n' -X POST $BASE/api/v1/admin/devices/ \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"EPSON LQ-630KII\",\"ip\":\"192.168.2.81\",\"hostname\":\"epson-printer\",\"model\":\"LQ-630KII\",\"protocol\":\"RAW\",\"factory_id\":\"$FACTORY_ID\",\"agent_id\":\"$AGENT_ID\",\"port\":9100}"

# 4. 验证
echo "=== 工厂列表 ==="
curl -s $BASE/api/v1/admin/factories/ -H "Authorization: Bearer $TOKEN"
echo ""
echo "=== Agent列表 ==="
curl -s $BASE/api/v1/admin/agents/ -H "Authorization: Bearer $TOKEN"
echo ""
echo "=== 设备列表 ==="
curl -s $BASE/api/v1/admin/devices/ -H "Authorization: Bearer $TOKEN"
echo ""
