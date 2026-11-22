// Shape of a magic-link token: "rfm_" + 32 random bytes in base64url.
export const magicLinkToken = /^rfm_[A-Za-z0-9_-]{43}$/;
