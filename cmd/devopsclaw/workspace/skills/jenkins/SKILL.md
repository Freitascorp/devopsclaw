---
name: jenkins
description: "Manage Jenkins CI/CD server using the `jenkins-cli` or REST API via curl. Jobs, builds, pipelines, and administration."
metadata: {"nanobot":{"emoji":"🎩","requires":{"bins":["curl"]}}}
---

# Jenkins Skill

Interact with Jenkins using its REST API via `curl` or the Jenkins CLI jar. Most operations use the JSON API.

## Authentication Setup

```bash
# Set these for all commands below
export JENKINS_URL="https://jenkins.example.com"
export JENKINS_USER="admin"
export JENKINS_TOKEN="your-api-token"  # generate at $JENKINS_URL/user/admin/configure
```

## Jobs & Builds

```bash
# List all jobs
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/api/json?tree=jobs[name,color]" | jq '.jobs[] | "\(.name): \(.color)"'

# Get job info
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/job/my-job/api/json" | jq '{name,color,lastBuild:.lastBuild.number}'

# Trigger a build
curl -s -X POST -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/job/my-job/build"

# Trigger with parameters
curl -s -X POST -u "$JENKINS_USER:$JENKINS_TOKEN" \
  "$JENKINS_URL/job/my-job/buildWithParameters?ENVIRONMENT=production&VERSION=v2.1.0"

# Get build status
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/job/my-job/lastBuild/api/json" \
  | jq '{number,result,duration,timestamp}'

# Get console output
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/job/my-job/lastBuild/consoleText" | tail -50

# Stop a build
curl -s -X POST -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/job/my-job/BUILD_NUMBER/stop"

# Get build history
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" \
  "$JENKINS_URL/job/my-job/api/json?tree=builds[number,result,timestamp,duration]{0,10}" \
  | jq '.builds[] | "\(.number): \(.result) (\(.duration/1000)s)"'
```

## Pipeline (Declarative Jenkinsfile)

```groovy
pipeline {
    agent any

    environment {
        DEPLOY_ENV = 'production'
    }

    stages {
        stage('Test') {
            steps {
                sh 'npm ci && npm test'
            }
        }
        stage('Build') {
            steps {
                sh 'docker build -t myapp:${BUILD_NUMBER} .'
            }
        }
        stage('Deploy') {
            when { branch 'main' }
            input { message 'Deploy to production?' }
            steps {
                sh './deploy.sh ${BUILD_NUMBER}'
            }
        }
    }

    post {
        failure {
            slackSend channel: '#deploys', message: "Build ${BUILD_NUMBER} failed"
        }
        success {
            slackSend channel: '#deploys', message: "Build ${BUILD_NUMBER} deployed"
        }
    }
}
```

## Nodes & Executors

```bash
# List nodes
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/computer/api/json" \
  | jq '.computer[] | {name:.displayName,offline,executors:.numExecutors}'

# Take node offline
curl -s -X POST -u "$JENKINS_USER:$JENKINS_TOKEN" \
  "$JENKINS_URL/computer/node-name/toggleOffline?offlineMessage=maintenance"

# Bring node online
curl -s -X POST -u "$JENKINS_USER:$JENKINS_TOKEN" \
  "$JENKINS_URL/computer/node-name/toggleOffline"
```

## Credentials

```bash
# List credentials
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" \
  "$JENKINS_URL/credentials/store/system/domain/_/api/json?tree=credentials[id,displayName]" \
  | jq '.credentials[]'
```

## Queue & System

```bash
# View build queue
curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/queue/api/json" \
  | jq '.items[] | {task:.task.name,why,inQueueSince}'

# Restart Jenkins (safe — waits for builds)
curl -s -X POST -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/safeRestart"

# Quiet down (stop accepting new builds)
curl -s -X POST -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/quietDown"
```

## Tips

- Generate API tokens at `$JENKINS_URL/user/YOUR_USER/configure`.
- Use `crumb` for CSRF protection: `curl -s -u "$JENKINS_USER:$JENKINS_TOKEN" "$JENKINS_URL/crumbIssuer/api/json"`.
- Append `/api/json` to any Jenkins URL to get its JSON representation.
- Use `?tree=` parameter to limit API response fields (faster).
- Use Blue Ocean UI for better pipeline visualization: `$JENKINS_URL/blue/`.
- Use `jenkins-cli.jar` for more operations: `java -jar jenkins-cli.jar -s $JENKINS_URL -auth $JENKINS_USER:$JENKINS_TOKEN help`.
