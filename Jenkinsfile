pipeline {
    agent any

    stages {
        stage('Cleanup Old Artifacts') {
            steps {
                script {
                    echo "Cleaning up any old visible Go installations to prevent testing conflicts..."
                    // This explicitly purges the old directory so 'go test ./...' won't try to test Go itself
                    sh 'rm -rf go_dist'
                }
            }
        }

        stage('Setup Go Environment') {
            steps {
                script {
                    // Prepend a dot (.) to the directory name to hide it from Go's recursive test runner
                    sh '''
                        if [ ! -d ".go_dist" ]; then
                            echo "Downloading Go 1.26.2..."
                            if command -v wget >/dev/null 2>&1; then
                                wget -q https://golang.org/dl/go1.26.2.linux-amd64.tar.gz
                            elif command -v curl >/dev/null 2>&1; then
                                curl -sSOL https://golang.org/dl/go1.26.2.linux-amd64.tar.gz
                            else
                                echo "No network utility found. Trying Python fallback..."
                                python3 -c "import urllib.request; urllib.request.urlretrieve('https://golang.org/dl/go1.26.2.linux-amd64.tar.gz', 'go1.26.2.linux-amd64.tar.gz')"
                            fi

                            mkdir -p .go_dist
                            tar -xzf go1.26.2.linux-amd64.tar.gz -C .go_dist --strip-components=1
                            rm go1.26.2.linux-amd64.tar.gz
                        else
                            echo "Cached Go binary distribution found in .go_dist"
                        fi
                    '''
                }
            }
        }

        stage('Test & Coverage') {
            steps {
                script {
                    // Prepend the isolated Go binary folder to the pipeline execution path
                    withEnv(["PATH=${WORKSPACE}/.go_dist/bin:${env.PATH}"]) {
                        sh 'go version'

                        // This will now exclusively run tests on your actual code packets
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
                timeout(time: 5, unit: 'MINUTES') {
                    // Waits for SonarQube callback to report passing/failing gates
                    waitForQualityGate abortPipeline: true
                }
            }
        }
    }

    post {
        always {
            echo "Pipeline complete. Coverage outputs preserved for SonarScanner upload."
        }
    }
}
