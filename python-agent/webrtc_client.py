import asyncio
import aiohttp
from aiortc import RTCPeerConnection, RTCSessionDescription, MediaStreamTrack
from aiortc.contrib.media import MediaBlackhole, MediaRecorder, MediaRelay
from loguru import logger
from typing import Optional, Callable
from config import settings


class WhipClient:
    """WebRTC client that connects to WHIP server"""

    def __init__(self, room_id: str):
        self.room_id = room_id
        self.pc: Optional[RTCPeerConnection] = None
        self.audio_track: Optional[MediaStreamTrack] = None
        self.on_track_callback: Optional[Callable] = None

    async def connect(self, audio_track: MediaStreamTrack) -> bool:
        """
        Connect to WHIP server and establish WebRTC connection

        Args:
            audio_track: Local audio track to send

        Returns:
            True if connection successful, False otherwise
        """
        try:
            # Create peer connection
            self.pc = RTCPeerConnection()
            self.audio_track = audio_track

            # Add audio track
            self.pc.addTrack(audio_track)

            # Handle incoming tracks (audio from other peer)
            @self.pc.on("track")
            async def on_track(track):
                logger.info(f"Received {track.kind} track from peer")
                if track.kind == "audio" and self.on_track_callback:
                    await self.on_track_callback(track)

            # Handle connection state changes
            @self.pc.on("connectionstatechange")
            async def on_connectionstatechange():
                logger.info(f"Connection state: {self.pc.connectionState}")
                if self.pc.connectionState == "failed":
                    await self.close()

            # Create offer
            offer = await self.pc.createOffer()
            await self.pc.setLocalDescription(offer)

            # Wait for ICE gathering to complete
            await self._wait_for_ice_gathering()

            # Send offer to WHIP server
            whip_url = f"http://{settings.server_host}:{settings.server_port}/whip?room={self.room_id}"

            async with aiohttp.ClientSession() as session:
                async with session.post(
                    whip_url,
                    data=self.pc.localDescription.sdp,
                    headers={"Content-Type": "application/sdp"}
                ) as response:
                    if response.status != 201:
                        logger.error(f"WHIP request failed: {response.status}")
                        return False

                    answer_sdp = await response.text()
                    logger.info("Received SDP answer from server")

            # Set remote description
            answer = RTCSessionDescription(sdp=answer_sdp, type="answer")
            await self.pc.setRemoteDescription(answer)

            logger.info(f"Successfully connected to room: {self.room_id}")
            return True

        except Exception as e:
            logger.error(f"Failed to connect: {e}")
            await self.close()
            return False

    async def _wait_for_ice_gathering(self):
        """Wait for ICE gathering to complete"""
        if self.pc.iceGatheringState == "complete":
            return

        # Wait for gathering complete event
        done = asyncio.Event()

        @self.pc.on("icegatheringstatechange")
        async def on_ice_gathering_state_change():
            if self.pc.iceGatheringState == "complete":
                done.set()

        # Wait with timeout
        try:
            await asyncio.wait_for(done.wait(), timeout=5.0)
        except asyncio.TimeoutError:
            logger.warning("ICE gathering timeout")

    def set_on_track_callback(self, callback: Callable):
        """Set callback for when remote track is received"""
        self.on_track_callback = callback

    async def close(self):
        """Close the peer connection"""
        if self.pc:
            await self.pc.close()
            logger.info("WebRTC connection closed")
