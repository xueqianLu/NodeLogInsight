#!/bin/bash

echo "正在连接到 MongoDB 容器..."
echo "------------------------------------"
echo "连接成功后，您将进入 MongoDB Shell。"
echo "以下是一些您可以尝试的查询示例："
echo
echo "1. 切换到 'node_logs' 数据库 (如果 docker compose exec 没有自动切换):"
echo "   use node_logs"
echo
echo "2. 查询 'committed_state' 集合中的任意一条记录:"
echo "   db.committed_state.findOne()"
echo
echo "3. 查询 'committed_state' 集合中某个特定高度的记录 (例如高度为 1000):"
echo "   db.committed_state.find({ height: 1000 })"
echo
echo "4. 统计 'committed_state' 集合中的文档总数:"
echo "   db.committed_state.countDocuments()"
echo
echo "5. 查询 'block_time_gap' 集合中最近的 5 条记录:"
echo "   db.block_time_gap.find().sort({ _id: -1 }).limit(5)"
echo
echo "6. 查询 'block_time_gap' 集合中 timeDiff 最大的N条记录 (例如N=5):"
echo "   db.block_time_gap.find().sort({ timeDiff: -1 }).limit(5)"
echo "------------------------------------"
echo

# 使用 docker-compose exec 命令进入 mongo 服务的容器，并启动 mongosh
# mongosh "mongodb://localhost:27017/node_logs" 会直接连接到指定的数据库
docker compose exec mongo mongosh "mongodb://localhost:27017/node_logs"