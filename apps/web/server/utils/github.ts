import { SignJWT, importPKCS8, importSPKI } from "jose"
import { createPrivateKey } from "crypto"

interface GitHubRepo {
  id: number
  name: string
  full_name: string
  default_branch: string
}

interface GitHubBranch {
  name: string
  commit: {
    sha: string
  }
}

interface GitHubInstallationToken {
  token: string
  expires_at: string
}

// Generate a JWT for GitHub App authentication
async function generateAppJWT(): Promise<string> {
  const config = useRuntimeConfig()

  const appId = config.githubAppId
  const privateKey = config.githubAppPrivateKey

  if (!appId || !privateKey) {
    console.error("GitHub App ID or Private Key not configured", appId, privateKey)
    throw new Error("GitHub App ID or Private Key not configured")
  }

  const now = Math.floor(Date.now() / 1000)

  // Ensure private key has proper line breaks
  let formattedKey = privateKey
  if (!privateKey.includes("\n")) {
    // If the key doesn't have line breaks, it might be escaped or single-line
    formattedKey = privateKey
      .replace(/\\n/g, "\n") // Replace escaped newlines
      .replace(/(-----BEGIN[^-]+-----)/g, "$1\n") // Add newline after BEGIN
      .replace(/(-----END[^-]+-----)/g, "\n$1") // Add newline before END
  }

  // Use Node.js crypto to handle both PKCS#1 and PKCS#8 formats
  const key = createPrivateKey(formattedKey)

  const jwt = await new SignJWT({})
    .setProtectedHeader({ alg: "RS256" })
    .setIssuedAt(now - 60)
    .setExpirationTime(now + 10 * 60)
    .setIssuer(appId)
    .sign(key)

  return jwt
}

// Get an installation access token
async function getInstallationToken(installationId: number): Promise<string> {
  const appJWT = await generateAppJWT()

  const response = await fetch(`https://api.github.com/app/installations/${installationId}/access_tokens`, {
    method: "POST",
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${appJWT}`,
      "X-GitHub-Api-Version": "2022-11-28",
    },
  })

  if (!response.ok) {
    const error = await response.text()
    throw new Error(`Failed to get installation token: ${response.status} ${error}`)
  }

  const data = (await response.json()) as GitHubInstallationToken
  return data.token
}

// Fetch repository information
export async function getRepository(
  installationId: number,
  owner: string,
  repo: string,
): Promise<{ id: number; defaultBranch: string }> {
  const token = await getInstallationToken(installationId)

  const response = await fetch(`https://api.github.com/repos/${owner}/${repo}`, {
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${token}`,
      "X-GitHub-Api-Version": "2022-11-28",
    },
  })

  if (!response.ok) {
    const error = await response.text()
    throw new Error(`Failed to fetch repository: ${response.status} ${error}`)
  }

  const data = (await response.json()) as GitHubRepo
  return {
    id: data.id,
    defaultBranch: data.default_branch,
  }
}

// Get the latest commit SHA from a branch
export async function getLatestCommitSHA(
  installationId: number,
  owner: string,
  repo: string,
  branch: string,
): Promise<string> {
  const token = await getInstallationToken(installationId)

  const response = await fetch(`https://api.github.com/repos/${owner}/${repo}/branches/${branch}`, {
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${token}`,
      "X-GitHub-Api-Version": "2022-11-28",
    },
  })

  if (!response.ok) {
    const error = await response.text()
    throw new Error(`Failed to fetch branch: ${response.status} ${error}`)
  }

  const data = (await response.json()) as GitHubBranch
  return data.commit.sha
}
