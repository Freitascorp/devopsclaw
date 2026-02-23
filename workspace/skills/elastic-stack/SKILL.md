```skill
---
name: elastic-stack
description: "Manage Elasticsearch, Kibana, and log pipelines. Index management, search queries, ILM policies, and Kibana dashboards via REST API."
metadata: {"nanobot":{"emoji":"🔍","requires":{"bins":["curl"]}}}
---

# Elastic Stack (ELK) Skill

Manage Elasticsearch and Kibana via REST API using `curl`. Covers indexing, searching, cluster health, ILM, and log management.

## Setup

```bash
export ES_URL="http://localhost:9200"
# Or with auth:
export ES_URL="https://elastic:password@elasticsearch.example.com:9200"
```

## Cluster Health

```bash
# Cluster health
curl -s "$ES_URL/_cluster/health" | jq '{status,number_of_nodes,active_shards,unassigned_shards}'

# Cluster health (color-coded)
curl -s "$ES_URL/_cluster/health?pretty"

# Node stats
curl -s "$ES_URL/_cat/nodes?v&h=name,heap.percent,cpu,load_1m,disk.used_percent"

# Cluster stats
curl -s "$ES_URL/_cluster/stats" | jq '{indices:.indices.count,docs:.indices.docs.count,store_size:.indices.store.size_in_bytes}'

# Pending tasks
curl -s "$ES_URL/_cluster/pending_tasks" | jq '.tasks'
```

## Index Management

```bash
# List indices
curl -s "$ES_URL/_cat/indices?v&s=index" | head -20

# List indices sorted by size
curl -s "$ES_URL/_cat/indices?v&s=store.size:desc" | head -10

# Create an index
curl -s -X PUT "$ES_URL/my-index" -H "Content-Type: application/json" -d '{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 1
  },
  "mappings": {
    "properties": {
      "timestamp": { "type": "date" },
      "message": { "type": "text" },
      "level": { "type": "keyword" },
      "service": { "type": "keyword" }
    }
  }
}'

# Delete an index
curl -s -X DELETE "$ES_URL/my-old-index"

# Get index mapping
curl -s "$ES_URL/my-index/_mapping" | jq '.[].mappings.properties'

# Get index settings
curl -s "$ES_URL/my-index/_settings" | jq '.[].settings.index'

# Reindex
curl -s -X POST "$ES_URL/_reindex" -H "Content-Type: application/json" -d '{
  "source": { "index": "old-index" },
  "dest": { "index": "new-index" }
}'

# Force merge (optimize)
curl -s -X POST "$ES_URL/my-index/_forcemerge?max_num_segments=1"
```

## Searching

```bash
# Simple search
curl -s "$ES_URL/logs-*/_search" -H "Content-Type: application/json" -d '{
  "query": { "match": { "message": "error" } },
  "size": 10,
  "sort": [{ "@timestamp": "desc" }]
}' | jq '.hits.hits[]._source'

# Search with time range
curl -s "$ES_URL/logs-*/_search" -H "Content-Type: application/json" -d '{
  "query": {
    "bool": {
      "must": [
        { "match": { "level": "ERROR" } },
        { "range": { "@timestamp": { "gte": "now-1h" } } }
      ],
      "filter": [
        { "term": { "service": "myapp" } }
      ]
    }
  },
  "size": 20
}' | jq '.hits.hits[]._source | {timestamp:.["@timestamp"],level,message}'

# Count documents
curl -s "$ES_URL/logs-*/_count" -H "Content-Type: application/json" -d '{
  "query": { "range": { "@timestamp": { "gte": "now-24h" } } }
}' | jq '.count'

# Aggregation (error count by service)
curl -s "$ES_URL/logs-*/_search" -H "Content-Type: application/json" -d '{
  "size": 0,
  "query": { "range": { "@timestamp": { "gte": "now-1h" } } },
  "aggs": {
    "by_service": {
      "terms": { "field": "service", "size": 20 },
      "aggs": {
        "errors": {
          "filter": { "term": { "level": "ERROR" } }
        }
      }
    }
  }
}' | jq '.aggregations.by_service.buckets[] | {service:.key,total:.doc_count,errors:.errors.doc_count}'
```

## Index Lifecycle Management (ILM)

```bash
# Create ILM policy
curl -s -X PUT "$ES_URL/_ilm/policy/logs-policy" -H "Content-Type: application/json" -d '{
  "policy": {
    "phases": {
      "hot": {
        "actions": {
          "rollover": { "max_size": "50gb", "max_age": "7d" }
        }
      },
      "warm": {
        "min_age": "7d",
        "actions": {
          "shrink": { "number_of_shards": 1 },
          "forcemerge": { "max_num_segments": 1 }
        }
      },
      "delete": {
        "min_age": "30d",
        "actions": { "delete": {} }
      }
    }
  }
}'

# Apply ILM policy to an index template
curl -s -X PUT "$ES_URL/_index_template/logs-template" -H "Content-Type: application/json" -d '{
  "index_patterns": ["logs-*"],
  "template": {
    "settings": {
      "index.lifecycle.name": "logs-policy",
      "index.lifecycle.rollover_alias": "logs"
    }
  }
}'

# Check ILM status
curl -s "$ES_URL/logs-*/_ilm/explain" | jq '.indices | to_entries[] | {index:.key,phase:.value.phase,age:.value.age}'
```

## Snapshots (Backup)

```bash
# Register a snapshot repository
curl -s -X PUT "$ES_URL/_snapshot/my-backup" -H "Content-Type: application/json" -d '{
  "type": "fs",
  "settings": { "location": "/mnt/backups/elasticsearch" }
}'

# Create a snapshot
curl -s -X PUT "$ES_URL/_snapshot/my-backup/snapshot-$(date +%Y%m%d)"

# List snapshots
curl -s "$ES_URL/_snapshot/my-backup/_all" | jq '.snapshots[] | {name:.snapshot,state,indices:.indices|length}'

# Restore a snapshot
curl -s -X POST "$ES_URL/_snapshot/my-backup/snapshot-20250101/_restore"
```

## Tips

- Use `_cat` APIs for human-readable output: `_cat/indices`, `_cat/nodes`, `_cat/shards`.
- Use `?v` with `_cat` APIs to include headers.
- Use ILM policies to automatically manage log retention and save storage.
- Use index templates so new indices get correct mappings and ILM policies automatically.
- For large clusters, use `_cat/shards?v&s=store:desc` to find hot shards.
- Use `_explain` API to debug why a document doesn't match a query.
```
