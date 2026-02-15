<script>
  export let agentName = 'Talon';

  // Get time-based greeting
  function getGreeting() {
    const hour = new Date().getHours();
    if (hour < 12) return 'Good morning';
    if (hour < 17) return 'Good afternoon';
    return 'Good evening';
  }

  // Get user name from system (or fallback)
  function getUserName() {
    return 'Bikram'; // TODO: get from backend user config
  }

  $: greeting = getGreeting();
  $: userName = getUserName();
</script>

<div class="welcome">
  <h1 class="welcome-heading">
    <span class="sparkle">&#10044;</span>
    <span>{greeting}, {userName}</span>
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

  .sparkle {
    color: #c4734b;
    font-size: 32px;
    line-height: 1;
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
