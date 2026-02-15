<script>
  import appIcon from '../assets/appicon.png';

  export let userName = '';

  // Get time-based greeting
  function getGreeting() {
    const hour = new Date().getHours();
    if (hour < 12) return 'Good morning';
    if (hour < 17) return 'Good afternoon';
    return 'Good evening';
  }

  $: greeting = getGreeting();
  $: displayGreeting = userName ? `${greeting}, ${userName}` : greeting;
</script>

<div class="welcome">
  <h1 class="welcome-heading">
    <img class="welcome-icon" src={appIcon} alt="Talon" />
    <span>{displayGreeting}</span>
  </h1>
</div>

<style>
  .welcome {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    min-height: 50vh;
    text-align: center;
    gap: 24px;
    padding-top: 10vh;
  }

  .welcome-heading {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 36px;
    font-weight: 600;
    color: var(--text-greeting);
    line-height: 1.2;
    letter-spacing: -0.5px;
  }

  .welcome-icon {
    width: 40px;
    height: 40px;
    border-radius: 10px;
    object-fit: cover;
    border: 2px solid var(--accent);
    box-shadow: 0 0 12px var(--accent-glow);
  }

  /* Subtle fade-in animation */
  .welcome {
    animation: welcomeFade 0.5s ease;
  }

  @keyframes welcomeFade {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
