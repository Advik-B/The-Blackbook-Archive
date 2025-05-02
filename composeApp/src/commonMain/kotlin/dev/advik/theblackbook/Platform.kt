package dev.advik.theblackbook

interface Platform {
    val name: String
}

expect fun getPlatform(): Platform