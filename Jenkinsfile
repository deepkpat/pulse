pipeline {
    agent any

    stages {
        stage('Setup Environment') {
            steps {
                sh 'bash scripts/setup.sh'
            }
        }

        stage('Test & Coverage') {
            steps {
                script {
                    // prepend the isolated Go binary folder to the pipeline execution path
                    withEnv(["PATH=${WORKSPACE}/.go_dist/bin:${env.PATH}"]) {
                        sh 'go version'
                        sh 'go test ./... -coverprofile=coverage.out'
                    }
                }
            }
        }

        stage('SonarQube Analysis') {
            steps {
                script {
                    def scannerHome = tool 'SonarScanner'
                    withSonarQubeEnv('SonarQube') {
                        sh "${scannerHome}/bin/sonar-scanner"
                    }
                }
            }
        }

        stage('Quality Gate') {
            steps {
                timeout(time: 4, unit: 'MINUTES') {
                    // waits for SonarQube callback to report passing/failing gates
                    waitForQualityGate abortPipeline: true
                }
            }
        }
    }

    post {
        always {
            echo "pipeline complete. overage outputs preserved for SonarScanner upload."
        }
    }
}
